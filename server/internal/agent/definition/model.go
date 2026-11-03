package definition

import "time"

// Status 表示 Agent 定义的生命周期状态。
type Status string

const (
	StatusDraft    Status = "draft"
	StatusActive   Status = "active"
	StatusOffline  Status = "offline"
	StatusArchived Status = "archived"
)

// AgentDefinition 表示 Agent 的稳定身份，具体运行配置由 AgentVersion 保存。
type AgentDefinition struct {
	ID          string
	WorkspaceID string
	Name        string
	Avatar      string
	Description string
	Tags        []string
	Visibility  string
	Status      Status
	IsDefault   bool
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AgentVersion 表示一次不可变的可运行配置快照。
type AgentVersion struct {
	ID             string
	AgentID        string
	VersionNumber  int
	Status         string
	PromptSnapshot []byte
	ModelSnapshot  []byte
	MemoryPolicy   []byte
	RuntimeLimits  []byte
	ConfigChecksum string
	CreatedBy      string
	PublishedAt    *time.Time
	CreatedAt      time.Time
}
