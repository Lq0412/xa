package memory

import "time"

// Memory 保存按 Agent 隔离的长期记忆。
type Memory struct {
	ID          string
	WorkspaceID string
	UserID      string
	AgentID     string
	Namespace   string
	MemoryType  string
	Content     string
	Source      string
	Confidence  float64
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpiresAt   *time.Time
}
