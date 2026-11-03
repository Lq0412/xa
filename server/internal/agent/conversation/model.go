package conversation

import (
	"encoding/json"
	"time"
)

// Status 表示会话是否仍可接收新轮次。
type Status string

const (
	StatusActive  Status = "active"
	StatusClosed  Status = "closed"
	StatusDeleted Status = "deleted"
)

// Channel 区分同一会话的文本和实时语音执行入口。
type Channel string

const (
	ChannelText     Channel = "text"
	ChannelRealtime Channel = "realtime"
)

// TurnStatus 记录可重查的执行结果，取消不表示整轮已交付。
type TurnStatus string

const (
	TurnRunning   TurnStatus = "running"
	TurnCompleted TurnStatus = "completed"
	TurnFailed    TurnStatus = "failed"
	TurnCanceled  TurnStatus = "canceled"
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
	DeletedAt      *time.Time
}

// Turn 通过 RequestID 去重，并记录实际使用的通道和终止状态。
type Turn struct {
	ID             string
	ConversationID string
	RequestID      string
	InputChecksum  string
	Channel        Channel
	Status         TurnStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// Message 是会话中的一条输入或输出消息。
type Message struct {
	ID             string
	ConversationID string
	TurnID         string
	Role           string
	Content        string
	ToolCalls      json.RawMessage
	ToolCallID     string
	AudioObjectKey string
	Completed      bool
	CreatedAt      time.Time
}
