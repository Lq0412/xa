// init-storage 是一次性存储初始化命令，不承担常驻业务进程职责。
package main

import (
	"context"
	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/framework/storage"
	"log/slog"
	"os"
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
	if err := storage.InitializeBucket(ctx, cfg.ObjectStore); err != nil {
		logger.Error("存储初始化失败", "error", err)
		os.Exit(1)
	}
	logger.Info("存储桶已就绪")
}
