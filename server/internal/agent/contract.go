package agent

// RequestScope 标识一次 Agent 请求的租户、用户、会话和追踪信息。
type RequestScope struct {
	WorkspaceID    string
	UserID         string
	AgentID        string
	AgentVersionID string
	ConversationID string
	TurnID         string
	RequestID      string
}
