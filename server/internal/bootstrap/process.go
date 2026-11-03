// Package bootstrap 组装进程依赖；未实现的业务接口不注册到运行进程。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/framework/config"
	httpserver "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/framework/storage"
)

func run(ctx context.Context, name, address string, cfg config.Config, logger *slog.Logger) (runErr error) {
	return runWithHandler(ctx, name, address, cfg, logger, nil)
}

type handlerFactory func(*pgxpool.Pool, *redisclient.Client, []httpserver.Check) (http.Handler, error)

func runWithHandler(ctx context.Context, name, address string, cfg config.Config, logger *slog.Logger, factory handlerFactory) (runErr error) {
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
	if err := postgres.Migrate(startupCtx, db); err != nil {
		return err
	}
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
	handler := httpserver.HealthHandler(checks, cfg.HealthTimeout, logger)
	if factory != nil {
		handler, err = factory(db, cache, checks)
		if err != nil {
			return err
		}
	}
	listener, err := (&net.ListenConfig{}).Listen(startupCtx, "tcp", address)
	if err != nil {
		return fmt.Errorf("监听 %s 地址失败: %w", name, err)
	}
	cancel()
	logger.Info("进程已启动", "process", name, "address", listener.Addr().String())
	handler = httpserver.WithRequestID(handler)
	return httpserver.Serve(ctx, listener, handler, cfg.ShutdownTimeout)
}
