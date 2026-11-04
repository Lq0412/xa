package bootstrap

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
	agentdefinition "linq-bs/server/internal/agent/definition"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	frameworkhttp "linq-bs/server/internal/framework/http"
	systemhttp "linq-bs/server/internal/system/transport/http"
	"linq-bs/server/internal/system/user"
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
	return runWithHandler(ctx, "server", s.config.HTTP.ServerAddress, s.config, s.logger, s.handler)
}

func (s *Server) handler(db *pgxpool.Pool, cache *redisclient.Client, checks []frameworkhttp.Check) (http.Handler, error) {
	users := user.NewPGRepository(db)
	if s.config.Admin.Email != "" {
		if err := users.EnsureAdmin(context.Background(), s.config.Admin.Email, s.config.Admin.Password); err != nil {
			return nil, err
		}
	}
	userService := user.NewService(users)
	sessions := auth.NewRedisSessionStore(cache, 24*time.Hour)
	resolver := auth.NewRedisResolver(sessions, func(ctx context.Context, userID string) (auth.Principal, error) {
		current, err := userService.Get(ctx, userID)
		if err != nil {
			return auth.Principal{}, err
		}
		if current.Status != "active" {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return auth.Principal{UserID: current.ID, WorkspaceID: current.WorkspaceID, Roles: []string{current.Role}}, nil
	})
	agents := agentdefinition.NewService(agentdefinition.NewPGRepository(db))
	authHandler := &systemhttp.Handler{Users: userService, Sessions: sessions}
	agentHandler := &agenthttp.ManagementHandler{Agents: agents}

	mux := http.NewServeMux()
	health := frameworkhttp.HealthHandler(checks, s.config.HealthTimeout, s.logger)
	mux.Handle("/health/", health)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.Handle("POST /api/v1/auth/logout", frameworkhttp.Authenticate(resolver, http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/v1/auth/me", frameworkhttp.Authenticate(resolver, http.HandlerFunc(authHandler.Me)))
	mux.Handle("GET /api/v1/available-agents", frameworkhttp.OptionalPrincipal(resolver, http.HandlerFunc(agentHandler.Available)))
	mux.Handle("GET /api/v1/agents", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.List)))
	mux.Handle("POST /api/v1/agents", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Create)))
	mux.Handle("GET /api/v1/agents/{agent_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Get)))
	mux.Handle("GET /api/v1/agents/{agent_id}/config", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.GetConfig)))
	mux.Handle("PUT /api/v1/agents/{agent_id}/config", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Save)))
	mux.Handle("POST /api/v1/agents/{agent_id}/enable", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Enable)))
	mux.Handle("POST /api/v1/agents/{agent_id}/offline", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Disable)))
	mux.Handle("DELETE /api/v1/agents/{agent_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(agentHandler.Delete)))
	return mux, nil
}
