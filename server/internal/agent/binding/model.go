package binding

// ModelBinding 固定能力使用的模型版本，不保存密钥。
type ModelBinding struct {
	Capability     string `json:"capability"`
	ModelProfileID string `json:"model_profile_id"`
	Revision       string `json:"revision"`
}

// ToolBinding 描述 Agent 版本绑定的工具及运行限制。
type ToolBinding struct {
	ToolVersionID string   `json:"tool_version_id"`
	Enabled       bool     `json:"enabled"`
	TimeoutMS     int64    `json:"timeout_ms"`
	MaxCalls      int      `json:"max_calls"`
	AllowedRoles  []string `json:"allowed_roles"`
	Configuration []byte   `json:"configuration"`
}

// KnowledgeBinding 描述 Agent 版本可访问的知识库。
type KnowledgeBinding struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Enabled         bool   `json:"enabled"`
	Filter          []byte `json:"filter"`
}

// EvaluationBinding 描述 Agent 版本使用的评价规则。
type EvaluationBinding struct {
	RubricID string `json:"rubric_id"`
	Enabled  bool   `json:"enabled"`
}
