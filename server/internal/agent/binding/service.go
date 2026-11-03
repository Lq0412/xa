package binding

import "context"

// Repository 只读取固定版本的绑定；写入与 AgentVersion 在同一事务完成。
type Repository interface {
	ListTools(ctx context.Context, workspaceID, versionID string) ([]ToolBinding, error)
	ListKnowledge(ctx context.Context, workspaceID, versionID string) ([]KnowledgeBinding, error)
	ListEvaluations(ctx context.Context, workspaceID, versionID string) ([]EvaluationBinding, error)
}
