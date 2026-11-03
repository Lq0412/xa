package definition

import (
	"encoding/json"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/prompt"
	"time"
)

// Status 表示 Agent 定义的生命周期状态。
type Status string

const (
	StatusActive  Status = "active"
	StatusOffline Status = "offline"
	StatusDeleted Status = "deleted"
)

// Visibility 控制配置可见性，不授予读取使用者私人会话的权限。
type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// AgentDefinition 表示 Agent 的稳定身份，具体运行配置由 AgentVersion 保存。
type AgentDefinition struct {
	ID               string
	WorkspaceID      string
	OwnerID          string
	OwnerType        string
	CurrentVersionID string
	Name             string
	Avatar           string
	Description      string
	Tags             []string
	Visibility       Visibility
	Status           Status
	IsDefault        bool
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// Configuration 保存运行配置，不包含用户会话或供应商凭证。
type Configuration struct {
	Prompts       []prompt.Slot
	Models        []binding.ModelBinding
	Tools         []binding.ToolBinding
	Knowledge     []binding.KnowledgeBinding
	Evaluations   []binding.EvaluationBinding
	MemoryPolicy  json.RawMessage
	RuntimeLimits json.RawMessage
}

// AgentVersion 表示一次不可变的可运行配置快照。
type AgentVersion struct {
	ID             string
	WorkspaceID    string
	AgentID        string
	VersionNumber  int
	Configuration  Configuration
	ConfigChecksum string
	CreatedBy      string
	CreatedAt      time.Time
}
