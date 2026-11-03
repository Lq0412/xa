package runtime

import "context"

// RuntimeContext 是一次运行的不可变边界，用户状态由外部持久化。
type RuntimeContext struct {
	Context        context.Context
	WorkspaceID    string
	UserID         string
	AgentID        string
	AgentVersionID string
	ConversationID string
	TurnID         string
	RequestID      string
}

// RunInput 是 AgentRunner 的统一输入。
type RunInput struct {
	Runtime RuntimeContext
	Message string
}
