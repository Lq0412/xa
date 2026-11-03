package definition

import (
	"context"
	"linq-bs/server/internal/framework/auth"
)

// Repository 隔离 Agent 定义和版本的持久化操作。
type Repository interface {
	Find(ctx context.Context, actor auth.Principal, agentID string) (AgentDefinition, error)
	List(ctx context.Context, actor auth.Principal) ([]AgentDefinition, error)
	FindVersion(ctx context.Context, actor auth.Principal, agentID, versionID string) (AgentVersion, error)
	// SaveRevision 在同一事务写入定义、完整版本及绑定，并比较旧版本指针。
	SaveRevision(ctx context.Context, actor auth.Principal, saved Saved, expectedVersionID string) error
	SetStatus(ctx context.Context, actor auth.Principal, agentID string, status Status) error
}
