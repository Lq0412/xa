package bootstrap

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/agent/conversation"
	agentdefinition "linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	agentrun "linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/infra/provider"
	systemhttp "linq-bs/server/internal/system/transport/http"
	"linq-bs/server/internal/system/user"
	"log/slog"
)

// Server 负责组装管理接口、用户接口和 Agent 运行依赖。
type Server struct {
	config config.Config
	logger *slog.Logger
	runs   *agentrun.RunService
}

// NewServer 创建 server 进程的依赖装配入口。
func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	return &Server{config: cfg, logger: logger}
}

// Run 启动 server 进程。
func (s *Server) Run(ctx context.Context) error {
	return runWithHandler(ctx, "server", s.config.HTTP.ServerAddress, s.config, s.logger, func(db *pgxpool.Pool, cache *redisclient.Client, checks []frameworkhttp.Check) (http.Handler, error) {
		h, err := s.handler(db, cache, checks)
		if err != nil {
			return nil, err
		}
		go (&agentrun.Sweeper{DB: db, Runs: s.runs, Interval: time.Duration(s.config.Run.SweepIntervalSec) * time.Second, Stale: time.Duration(s.config.Run.StaleAfterSec) * time.Second}).Run(ctx)
		return h, nil
	})
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
	agents := agentdefinition.NewService(agentdefinition.NewPGRepository(db), s.config.ChatModels)
	model := provider.NewBailian(s.config.Bailian.APIKey, s.config.Bailian.BaseURL)
	var chatProvider provider.ChatProvider = model
	memories := &memory.Service{DB: db, Agents: agents, Model: model}
	registry, err := NewToolRegistry(memories, db)
	if err != nil {
		return nil, err
	}
	agents.ToolValidator = registry.ValidateBindings
	runStore := agentrun.NewPGStore(db)
	runSvc := &agentrun.RunService{
		DB: db, Store: runStore, Registry: registry, Provider: chatProvider, Memory: memories, Agents: agents,
		Engine: &agentrun.NativeEngine{}, Redis: cache, RedisOK: func() bool { return cache != nil },
		Config: s.config.Run, Coord: agentrun.NewCoordinator(), InstanceID: "server",
		SnapshotParts: conversation.SnapshotRunParts,
	}
	s.runs = runSvc
	authHandler := &systemhttp.Handler{Users: userService, Sessions: sessions}
	agentHandler := &agenthttp.ManagementHandler{Agents: agents, AllowedModels: s.config.ChatModels}
	libraryHandler := &agenthttp.LibraryHandler{Library: &agentdefinition.Library{DB: db}}
	toolsHandler := &agenthttp.ToolsHandler{Registry: registry, Agents: agents}
	memoryHandler := &agenthttp.MemoryHandler{Memory: memories, Agents: agents}
	convSvc := &conversation.Service{
		DB: db, Agents: agents, Model: model, Provider: chatProvider, Memory: memories,
		AllowedModels: s.config.ChatModels, Runs: runSvc, Registry: registry,
	}
	conversationHandler := &agenthttp.ConversationHandler{Conversations: convSvc}
	runHandler := &agenthttp.RunHandler{Conversations: convSvc, Runs: runSvc}

	mux := http.NewServeMux()
	registerRoutes(mux, resolver, routeHandlers{
		auth: authHandler, agents: agentHandler, library: libraryHandler, tools: toolsHandler,
		memory: memoryHandler, conversations: conversationHandler, runs: runHandler,
		health: frameworkhttp.HealthHandler(checks, s.config.HealthTimeout, s.logger),
	})
	handler := http.Handler(mux)
	if raw := strings.TrimSpace(os.Getenv("XA_HTTP_CORS_ORIGINS")); raw != "" {
		handler = frameworkhttp.WithAllowOrigins(strings.Split(raw, ","), mux)
	}
	return handler, nil
}

// routeMux 保留标准库路由注册边界，也让覆盖测试记录实际注册的方法和路径。
type routeMux interface {
	http.Handler
	Handle(string, http.Handler)
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}

type routeHandlers struct {
	auth          *systemhttp.Handler
	agents        *agenthttp.ManagementHandler
	library       *agenthttp.LibraryHandler
	tools         *agenthttp.ToolsHandler
	memory        *agenthttp.MemoryHandler
	conversations *agenthttp.ConversationHandler
	runs          *agenthttp.RunHandler
	health        http.Handler
}

// registerRoutes 只登记接口，不访问存储；生产与文档导出共用此入口。
func registerRoutes(mux routeMux, resolver auth.Resolver, handlers routeHandlers) huma.API {
	api := frameworkhttp.NewAPI(mux)
	handlers.auth.RegisterRoutes(api, resolver)
	handlers.agents.RegisterRoutes(api, resolver)
	memoryHandler, libraryHandler, toolsHandler := handlers.memory, handlers.library, handlers.tools
	conversationHandler, runHandler := handlers.conversations, handlers.runs
	mux.Handle("GET /api/v1/memories", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.List)))
	mux.Handle("POST /api/v1/memories", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Add)))
	mux.Handle("PUT /api/v1/memories/{memory_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Edit)))
	mux.Handle("DELETE /api/v1/memories/{memory_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Delete)))
	mux.Handle("POST /api/v1/memories/{memory_id}/restore", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Restore)))
	mux.Handle("GET /api/v1/memories/{memory_id}/events", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Events)))
	mux.Handle("GET /api/v1/agents/{agent_id}/memory", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Core)))
	mux.Handle("PUT /api/v1/agents/{agent_id}/memory-policy", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Policy)))
	mux.Handle("GET /api/v1/memory/stats", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Stats)))
	mux.Handle("POST /api/v1/memory/jobs/{job_id}/retry", frameworkhttp.Authenticate(resolver, http.HandlerFunc(memoryHandler.Retry)))
	mux.Handle("/health/", handlers.health)
	mux.Handle("GET /api/v1/agent-library", frameworkhttp.Authenticate(resolver, http.HandlerFunc(libraryHandler.List)))
	mux.Handle("PUT /api/v1/agent-library/{agent_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(libraryHandler.Add)))
	mux.Handle("GET /api/v1/tools", frameworkhttp.Authenticate(resolver, http.HandlerFunc(toolsHandler.Catalog)))
	mux.Handle("GET /api/v1/agents/{agent_id}/tools", frameworkhttp.Authenticate(resolver, http.HandlerFunc(toolsHandler.GetAgentTools)))
	mux.Handle("PUT /api/v1/agents/{agent_id}/tools", frameworkhttp.Authenticate(resolver, http.HandlerFunc(toolsHandler.PutAgentTools)))
	mux.Handle("POST /api/v1/conversations", frameworkhttp.Authenticate(resolver, http.HandlerFunc(conversationHandler.Create)))
	mux.Handle("GET /api/v1/conversations", frameworkhttp.Authenticate(resolver, http.HandlerFunc(conversationHandler.List)))
	mux.Handle("GET /api/v1/conversations/{conversation_id}/messages", frameworkhttp.Authenticate(resolver, http.HandlerFunc(conversationHandler.Messages)))
	mux.Handle("GET /api/v1/conversations/{conversation_id}/summary", frameworkhttp.Authenticate(resolver, http.HandlerFunc(conversationHandler.Summary)))
	mux.Handle("POST /api/v1/conversations/{conversation_id}/messages", frameworkhttp.Authenticate(resolver, http.HandlerFunc(conversationHandler.Send)))
	mux.Handle("GET /api/v1/runs/{run_id}/events", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.Events)))
	mux.Handle("POST /api/v1/runs/{run_id}/cancel", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.Cancel)))
	mux.Handle("POST /api/v1/approvals/{approval_id}/decision", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.Decision)))
	mux.Handle("GET /api/v1/admin/approvals", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.ApprovalRecords)))
	mux.Handle("GET /api/v1/approvals", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.UserApprovals)))
	mux.Handle("GET /api/v1/agents/{agent_id}/activity", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.AgentActivity)))
	mux.Handle("GET /api/v1/runs", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.ListRuns)))
	mux.Handle("GET /api/v1/runs/{run_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.GetRun)))
	mux.Handle("GET /api/v1/runs/{run_id}/spans", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.Spans)))
	mux.Handle("GET /api/v1/runs/{run_id}/tool-results/{call_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.ToolResult)))
	mux.Handle("GET /api/v1/admin/runs", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.ListRuns)))
	mux.Handle("GET /api/v1/admin/runs/{run_id}", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.GetRun)))
	mux.Handle("GET /api/v1/admin/runs/{run_id}/spans", frameworkhttp.Authenticate(resolver, http.HandlerFunc(runHandler.Spans)))
	return api
}

// OpenAPI 从同一注册入口导出约定，不加载配置、不连接数据库，也不启动监听器。
func OpenAPI() *huma.OpenAPI {
	return registerRoutes(http.NewServeMux(), nil, documentationHandlers()).OpenAPI()
}

func documentationHandlers() routeHandlers {
	return routeHandlers{
		auth: &systemhttp.Handler{}, agents: &agenthttp.ManagementHandler{},
		library: &agenthttp.LibraryHandler{}, tools: &agenthttp.ToolsHandler{},
		memory: &agenthttp.MemoryHandler{}, conversations: &agenthttp.ConversationHandler{},
		runs: &agenthttp.RunHandler{}, health: frameworkhttp.HealthHandler(nil, time.Second, slog.Default()),
	}
}
