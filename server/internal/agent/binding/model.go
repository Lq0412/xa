package binding

import "encoding/json"

// ModelBinding 固定能力使用的模型版本，不保存密钥。
type ModelBinding struct {
	Capability     string `json:"capability"`
	ModelProfileID string `json:"model_profile_id"`
	Revision       string `json:"revision"`
}

// ToolBinding 描述 Agent 版本绑定的工具及运行限制。
type ToolBinding struct {
	Name          string          `json:"name"`
	ToolVersionID string          `json:"tool_version_id,omitempty"`
	Enabled       bool            `json:"enabled"`
	Approval      string          `json:"approval,omitempty"`
	TimeoutMS     int64           `json:"timeout_ms,omitempty"`
	MaxCalls      int             `json:"max_calls,omitempty"`
	AllowedRoles  []string        `json:"allowed_roles,omitempty"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

// KnowledgeBinding 描述 Agent 版本可访问的知识库。
type KnowledgeBinding struct {
	KnowledgeBaseID string `json:"knowledge_base_id" doc:"知识库 ID；保存时校验所有权及可见性，检索时再次检查当前权限"`
	Enabled         bool   `json:"enabled"`
	Filter          []byte `json:"filter" doc:"保留字段，目前必须为空；不支持过滤表达式"`
}

// EvaluationBinding 描述 Agent 版本使用的评价规则。
type EvaluationBinding struct {
	RubricID string `json:"rubric_id"`
	Enabled  bool   `json:"enabled"`
}
