package bootstrap

import (
	"context"
	"linq-bs/server/internal/framework/config"
	"log/slog"
)

// Server 负责组装管理接口、用户接口和 Agent 运行依赖。
type Server struct {
	config config.Config
	logger *slog.Logger
}

// NewServer 创建 server 进程的依赖装配入口。
func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	return &Server{config: cfg, logger: logger}
}

// Run 启动 server 进程。
func (s *Server) Run(ctx context.Context) error {
	return run(ctx, "server", s.config.HTTP.ServerAddress, s.config, s.logger)
}
