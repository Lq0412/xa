package conversation

import "time"

// Status 表示会话是否仍可接收新轮次。
type Status string

const (
	StatusActive Status = "active"
	StatusClosed Status = "closed"
)

// Conversation 创建后固定绑定一个 Agent 版本。
type Conversation struct {
	ID             string
	WorkspaceID    string
	UserID         string
	AgentID        string
	AgentVersionID string
	Title          string
	Status         Status
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Message 是会话中的一条输入或输出消息。
type Message struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	CreatedAt      time.Time
}
