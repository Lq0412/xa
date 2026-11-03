package memory

import "context"

// Service 提供记忆读取和写入用例。
type Service interface {
	Load(ctx context.Context, workspaceID string, userID string, agentID string, namespace string) ([]Memory, error)
	Store(ctx context.Context, item Memory) error
}
