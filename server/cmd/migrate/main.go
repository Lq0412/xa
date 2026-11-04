package main

import (
	"context"
	"log/slog"
	"os"

	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/framework/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()
	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("连接 PostgreSQL 失败", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := postgres.Migrate(ctx, db); err != nil {
		logger.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
	logger.Info("数据库迁移完成")
}
