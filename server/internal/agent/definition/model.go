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
	OwnerUserID      string     `json:"owner_user_id,omitempty"`
	OwnerType        string     `json:"owner_type"`
	CurrentVersionID string     `json:"current_version_id"`
	Name             string     `json:"name"`
	Avatar           string     `json:"avatar"`
	Description      string     `json:"description"`
	Tags             []string   `json:"tags"`
	Visibility       Visibility `json:"visibility" enum:"public,private"`
	Status           Status     `json:"status"`
	IsDefault        bool       `json:"is_default"`
	IsPrimary        bool       `json:"is_primary"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// Configuration 保存运行配置，不包含用户会话或供应商凭证。
type Configuration struct {
	SoulMD string `json:"soul_md"`
	Model  string `json:"model"`
	// Prompts 仅接收旧管理端输入，持久化使用 SoulMD。
	Prompts       []prompt.Slot               `json:"prompts,omitempty"`
	Models        []binding.ModelBinding      `json:"models"`
	Tools         []binding.ToolBinding       `json:"tools"`
	Knowledge     []binding.KnowledgeBinding  `json:"knowledge"`
	Evaluations   []binding.EvaluationBinding `json:"evaluations"`
	MemoryPolicy  json.RawMessage             `json:"memory_policy"`
	RuntimeLimits json.RawMessage             `json:"runtime_limits"`
	// EnableThinking 为 nil 时对话默认开启深度思考。
	EnableThinking *bool `json:"enable_thinking,omitempty"`
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
