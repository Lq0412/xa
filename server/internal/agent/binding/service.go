package binding

import "context"

// Repository 隔离 Agent 版本的工具、知识库和评价绑定。
type Repository interface {
	ListTools(ctx context.Context, versionID string) ([]ToolBinding, error)
	ListKnowledge(ctx context.Context, versionID string) ([]KnowledgeBinding, error)
	ListEvaluations(ctx context.Context, versionID string) ([]EvaluationBinding, error)
	SaveTools(ctx context.Context, versionID string, bindings []ToolBinding) error
}
