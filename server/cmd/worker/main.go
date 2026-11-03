package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	// 后续任务领取循环应使用同一根 context，终止时停止领取新任务。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bootstrap.NewWorker(cfg, logger).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("worker 退出失败", "error", err)
		os.Exit(1)
	}
	logger.Info("worker 已正常退出")
}
