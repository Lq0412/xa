package catalog

import (
	"context"

	"linq-bs/server/internal/agent/binding"
)

// Catalog 根据 Agent 版本绑定生成本次运行可用的工具目录。
type Catalog interface {
	Build(ctx context.Context, bindings []binding.ToolBinding) (Snapshot, error)
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
