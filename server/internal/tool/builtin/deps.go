package builtin

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/memory"
)

// Deps 是内置工具的运行时依赖，由启动层注入。
type Deps struct {
	Memory *memory.Service
	DB     *pgxpool.Pool
}
