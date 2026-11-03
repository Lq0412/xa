package runtime

import "linq-bs/server/internal/infra/model"

// RuntimeContext 是一次运行的不可变边界，用户状态由外部持久化。
type RuntimeContext struct {
	WorkspaceID    string
	UserID         string
	AgentID        string
	AgentVersionID string
	ConversationID string
	TurnID         string
	RequestID      string
}

// ContextSnapshot 只属于本次运行，禁止保存在共享 Agent 实例中。
type ContextSnapshot struct {
	History       []model.Message
	MemorySummary string
	Knowledge     []string
}

// RunInput 是文本轮次输入；context.Context 通过方法参数传递。
type RunInput struct {
	Runtime  RuntimeContext
	Provider ProviderInput
	Context  ContextSnapshot
	Message  string
}
