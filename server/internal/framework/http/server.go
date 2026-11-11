// Package http 提供 HTTP 生命周期和基础健康检查。
package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Check 对真实依赖执行有超时的可用性检查。
type Check struct {
	Name string
	Ping func(context.Context) error
}

// HealthHandler 区分进程存活与依赖就绪，不向外返回连接参数或底层错误。
func HealthHandler(checks []Check, timeout time.Duration, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	RegisterHealthRoutes(NewAPI(mux), checks, timeout, logger)
	return mux
}

type healthLive struct {
	Status string `json:"status" doc:"进程状态" enum:"ok"`
}

type healthReady struct {
	Status       string            `json:"status" doc:"依赖就绪状态" enum:"ok,unavailable"`
	Dependencies map[string]string `json:"dependencies" doc:"各依赖的状态"`
}

type liveOutput struct{ Body healthLive }
type readyOutput struct {
	Status int
	Body   healthReady
}

// RegisterHealthRoutes 为 server、worker 和离线导出共用健康接口约定。
func RegisterHealthRoutes(api huma.API, checks []Check, timeout time.Duration, logger *slog.Logger) {
	huma.Register(api, Operation(api, "健康检查", "healthLive", "GET", "/health/live", "检查进程存活", 200, 0, nil),
		func(context.Context, *EmptyInput) (*liveOutput, error) {
			return &liveOutput{Body: healthLive{Status: "ok"}}, nil
		})
	op := Operation(api, "健康检查", "healthReady", "GET", "/health/ready", "检查依赖就绪", 200, 0, nil)
	op.Responses["503"] = &huma.Response{Description: "依赖不可用", Content: map[string]*huma.MediaType{
		"application/json": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[healthReady](), true, "HealthReady")},
	}}
	huma.Register(api, op, func(ctx context.Context, _ *EmptyInput) (*readyOutput, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		status := http.StatusOK
		dependencies := make(map[string]string, len(checks))
		for _, check := range checks {
			dependencies[check.Name] = "ok"
			if err := check.Ping(ctx); err != nil {
				status = http.StatusServiceUnavailable
				dependencies[check.Name] = "unavailable"
				logger.Warn("依赖健康检查失败", "dependency", check.Name, "error", err)
			}
		}
		state := "ok"
		if status != http.StatusOK {
			state = "unavailable"
		}
		return &readyOutput{Status: status, Body: healthReady{Status: state, Dependencies: dependencies}}, nil
	})
}

// Serve 在调用方提供的监听器上运行，取消时停止接收请求并等待在途请求结束。
// 未来注册 WebSocket 时，还需由连接管理器显式关闭升级后的连接。
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, shutdownTimeout time.Duration) error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
		BaseContext:    func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		// 根 context 已取消，关闭等待需要独立的有界 context。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
		serveErr := <-done
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(err, serveErr)
	}
}
