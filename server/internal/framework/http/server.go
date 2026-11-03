// Package http 提供 HTTP 生命周期和基础健康检查。
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Check 对真实依赖执行有超时的可用性检查。
type Check struct {
	Name string
	Ping func(context.Context) error
}

// HealthHandler 区分进程存活与依赖就绪，不向外返回连接参数或底层错误。
func HealthHandler(checks []Check, timeout time.Duration, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}, logger)
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
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
		writeJSON(w, status, struct {
			Status       string            `json:"status"`
			Dependencies map[string]string `json:"dependencies"`
		}{state, dependencies}, logger)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Warn("写入健康检查响应失败", "error", err)
	}
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
