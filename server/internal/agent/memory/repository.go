package memory

import "context"

// Repository 保证记忆查询始终包含 workspace、user 和 agent 作用域。
type Repository interface {
	List(ctx context.Context, workspaceID string, userID string, agentID string, namespace string) ([]Memory, error)
	Save(ctx context.Context, item Memory) error
	Delete(ctx context.Context, memoryID string) error
}
