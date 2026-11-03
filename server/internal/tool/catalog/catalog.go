package catalog

import (
	"context"

	"encoding/json"
	"linq-bs/server/internal/framework/auth"
	"time"
)

// Catalog 校验当前主体的权限和工具版本，不依赖 Agent 的领域对象。
type Catalog interface {
	Build(ctx context.Context, actor auth.Principal, selections []Selection) (Snapshot, error)
}

// Selection 由调用方转换绑定生成，固定版本不能跳过当前权限检查。
type Selection struct {
	VersionID     string
	Timeout       time.Duration
	MaxCalls      int
	AllowedRoles  []string
	Configuration json.RawMessage
}

// Snapshot 是经过权限、版本和参数 Schema 校验的工具目录。
type Snapshot struct {
	Tools    []Tool
	Checksum string
}

// Tool 表示一个可被 ReAct Agent 调用的工具。
type Tool struct {
	VersionID string
	Name      string
	Schema    []byte
}
