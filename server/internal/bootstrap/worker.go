package bootstrap

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/framework/config"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/infra/provider"
	"log/slog"
	"net/http"
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
	return runWithHandler(ctx, "worker", w.config.HTTP.WorkerAddress, w.config, w.logger, func(db *pgxpool.Pool, _ *redisclient.Client, checks []frameworkhttp.Check) (http.Handler, error) {
		return frameworkhttp.HealthHandler(checks, w.config.HealthTimeout, w.logger), nil
	}, func(taskCtx context.Context, db *pgxpool.Pool) {
		service := &memory.Service{DB: db, Agents: definition.NewService(definition.NewPGRepository(db), w.config.ChatModels), Model: provider.NewBailian(w.config.Bailian.APIKey, w.config.Bailian.BaseURL)}
		service.Run(taskCtx, w.logger)
	})
}
