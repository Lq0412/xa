package bootstrap

import (
	"context"
	"linq-bs/server/internal/framework/config"
	"log/slog"
)

// Worker 负责组装异步评价、记忆提取和知识摄取任务。
type Worker struct {
	config config.Config
	logger *slog.Logger
}

// NewWorker 创建 worker 进程的依赖装配入口。
func NewWorker(cfg config.Config, logger *slog.Logger) *Worker {
	return &Worker{config: cfg, logger: logger}
}

// Run 启动 worker 进程。
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("worker 尚未注册业务任务处理器")
	return run(ctx, "worker", w.config.HTTP.WorkerAddress, w.config, w.logger)
}
