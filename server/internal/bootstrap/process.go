// Package bootstrap 组装进程依赖；未实现的业务接口不注册到运行进程。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"linq-bs/server/internal/framework/config"
	httpserver "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/framework/storage"
)

func run(ctx context.Context, name, address string, cfg config.Config, logger *slog.Logger) (runErr error) {
	if err := cfg.Validate(); err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()

	db, err := postgres.Open(startupCtx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()
	cache, err := redis.Open(startupCtx, cfg.Redis)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, cache.Close()) }()
	objects, err := storage.Open(startupCtx, cfg.ObjectStore)
	if err != nil {
		return err
	}
	defer objects.Close()

	checks := []httpserver.Check{
		{Name: "postgres", Ping: db.Ping},
		{Name: "redis", Ping: func(ctx context.Context) error { return cache.Ping(ctx).Err() }},
		{Name: "object_store", Ping: objects.Ping},
	}
	listener, err := (&net.ListenConfig{}).Listen(startupCtx, "tcp", address)
	if err != nil {
		return fmt.Errorf("监听 %s 地址失败: %w", name, err)
	}
	cancel()
	logger.Info("进程已启动", "process", name, "address", listener.Addr().String())
	return httpserver.Serve(ctx, listener, httpserver.HealthHandler(checks, cfg.HealthTimeout, logger), cfg.ShutdownTimeout)
}
