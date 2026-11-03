package runtime

import (
	"context"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/tool/catalog"
)

// Agent 是 Eino ChatModelAgent 在平台内部的适配接口。
type Agent interface {
	Run(ctx context.Context, input RunInput) (<-chan Event, error)
}

// AgentProvider 根据不可变配置获取可复用的 Agent 实例。
type AgentProvider interface {
	Get(ctx context.Context, input ProviderInput) (Agent, error)
}

// ProviderInput 提供不可变配置、解析结果和缓存指纹，不包含用户记忆或历史。
type ProviderInput struct {
	Version           definition.AgentVersion
	Prompt            string
	Tools             catalog.Snapshot
	ModelRevision     string
	KnowledgeRevision string
}

// AgentRunner 执行文本轮次；取消由 ctx 传递，结束时必须关闭事件通道。
type AgentRunner interface {
	Run(ctx context.Context, input RunInput) (<-chan Event, error)
}
