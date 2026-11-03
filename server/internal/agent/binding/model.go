package binding

// ModelBinding 固定能力使用的模型版本，不保存密钥。
type ModelBinding struct {
	Capability     string
	ModelProfileID string
	Revision       string
}

// ToolBinding 描述 Agent 版本绑定的工具及运行限制。
type ToolBinding struct {
	ToolVersionID string
	Enabled       bool
	TimeoutMS     int64
	MaxCalls      int
	AllowedRoles  []string
	Configuration []byte
}

// KnowledgeBinding 描述 Agent 版本可访问的知识库。
type KnowledgeBinding struct {
	KnowledgeBaseID string
	Enabled         bool
	Filter          []byte
}

// EvaluationBinding 描述 Agent 版本使用的评价规则。
type EvaluationBinding struct {
	RubricID string
	Enabled  bool
}
