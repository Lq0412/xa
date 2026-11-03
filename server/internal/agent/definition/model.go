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
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	OwnerID          string     `json:"owner_id"`
	OwnerType        string     `json:"owner_type"`
	CurrentVersionID string     `json:"current_version_id"`
	Name             string     `json:"name"`
	Avatar           string     `json:"avatar"`
	Description      string     `json:"description"`
	Tags             []string   `json:"tags"`
	Visibility       Visibility `json:"visibility"`
	Status           Status     `json:"status"`
	IsDefault        bool       `json:"is_default"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// Configuration 保存运行配置，不包含用户会话或供应商凭证。
type Configuration struct {
	Prompts       []prompt.Slot               `json:"prompts"`
	Models        []binding.ModelBinding      `json:"models"`
	Tools         []binding.ToolBinding       `json:"tools"`
	Knowledge     []binding.KnowledgeBinding  `json:"knowledge"`
	Evaluations   []binding.EvaluationBinding `json:"evaluations"`
	MemoryPolicy  json.RawMessage             `json:"memory_policy"`
	RuntimeLimits json.RawMessage             `json:"runtime_limits"`
}

// AgentVersion 表示一次不可变的可运行配置快照。
type AgentVersion struct {
	ID             string        `json:"id"`
	WorkspaceID    string        `json:"workspace_id"`
	AgentID        string        `json:"agent_id"`
	VersionNumber  int           `json:"version_number"`
	Configuration  Configuration `json:"configuration"`
	ConfigChecksum string        `json:"config_checksum"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}
