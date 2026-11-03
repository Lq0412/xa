package definition

import "context"

// Repository 隔离 Agent 定义和版本的持久化操作。
type Repository interface {
	Find(ctx context.Context, agentID string) (AgentDefinition, error)
	List(ctx context.Context, workspaceID string) ([]AgentDefinition, error)
	Save(ctx context.Context, definition AgentDefinition) error
	FindVersion(ctx context.Context, versionID string) (AgentVersion, error)
	SaveVersion(ctx context.Context, version AgentVersion) error
}
