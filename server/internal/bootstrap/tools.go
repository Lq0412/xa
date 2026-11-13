package bootstrap

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/rag"
	"linq-bs/server/internal/tool"
	"linq-bs/server/internal/tool/builtin"
)

// NewToolRegistry 创建并装配生产环境工具注册表。
func NewToolRegistry(mem *memory.Service, db *pgxpool.Pool, knowledge *rag.Searcher) (*tool.Registry, error) {
	r := tool.NewRegistry()
	if err := builtin.RegisterProduction(r, builtin.Deps{Memory: mem, DB: db, Knowledge: knowledge}); err != nil {
		return nil, fmt.Errorf("装配内置工具: %w", err)
	}
	return r, nil
}
