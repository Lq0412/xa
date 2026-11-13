//go:build integration

package bootstrap_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/rag"
)

// newChatService 按生产 server 装配对话 Send 所需的 RunService 与工具注册表。
func newChatService(t *testing.T, db *pgxpool.Pool, agents definition.Service, mem *memory.Service) *conversation.Service {
	t.Helper()
	cfg := loadConfig(t)
	model := provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)
	if mem == nil {
		mem = &memory.Service{DB: db, Agents: agents, Model: model, QueryEmbed: func(_ context.Context, query string) ([]float32, error) {
			return integrationVector(query), nil
		}}
	}
	knowledge := &rag.Service{DB: db, Config: cfg.Knowledge}
	search, err := rag.NewSearcher(knowledge, agents, model)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := bootstrap.NewToolRegistry(mem, db, search)
	if err != nil {
		t.Fatal(err)
	}
	if impl, ok := agents.(*definition.ServiceImpl); ok {
		impl.ToolValidator = reg.ValidateBindings
		impl.KnowledgeValidator = knowledge.ValidateBindings
	}
	runSvc := &run.RunService{
		DB: db, Store: run.NewPGStore(db), Registry: reg, Provider: model,
		Memory: mem, Engine: &run.NativeEngine{}, Coord: run.NewCoordinator(), InstanceID: "integration",
	}
	wireRunRedis(t, cfg, runSvc)
	return &conversation.Service{
		DB: db, Agents: agents, Model: model, Provider: model, Memory: mem,
		AllowedModels: cfg.ChatModels, Runs: runSvc, Registry: reg,
	}
}

func wireRunRedis(t *testing.T, cfg config.Config, runSvc *run.RunService) {
	t.Helper()
	cache, err := redis.Open(context.Background(), cfg.Redis)
	if err != nil {
		t.Fatal(err)
	}
	runSvc.Redis = cache
	runSvc.RedisOK = func() bool {
		return cache.Ping(context.Background()).Err() == nil
	}
}

func newChatServiceBadRedis(t *testing.T, db *pgxpool.Pool, agents definition.Service, mem *memory.Service) *conversation.Service {
	t.Helper()
	svc := newChatService(t, db, agents, mem)
	svc.Runs.Redis = nil
	svc.Runs.RedisOK = func() bool { return false }
	return svc
}
