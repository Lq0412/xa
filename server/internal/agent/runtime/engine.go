package runtime

import "context"

// Agent 是 Eino ChatModelAgent 在平台内部的适配接口。
type Agent interface {
	Run(ctx context.Context, input string) (<-chan Event, error)
}

// AgentProvider 根据不可变配置获取可复用的 Agent 实例。
type AgentProvider interface {
	Get(ctx context.Context, input ProviderInput) (Agent, error)
}

// ProviderInput 是运行实例缓存的组成部分。
type ProviderInput struct {
	AgentVersionID      string
	ConfigChecksum      string
	ToolCatalogChecksum string
	ModelRevision       string
	KnowledgeRevision   string
}

// AgentRunner 执行一次会话轮次并输出平台事件。
type AgentRunner interface {
	Run(ctx context.Context, input RunInput) (<-chan Event, error)
}
