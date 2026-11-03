package definition

import "context"

// Service 提供 Agent 创建、配置校验、版本发布和下线用例。
type Service interface {
	Create(ctx context.Context, definition AgentDefinition) (AgentDefinition, error)
	Get(ctx context.Context, agentID string) (AgentDefinition, error)
	List(ctx context.Context, workspaceID string) ([]AgentDefinition, error)
	ValidateDraft(ctx context.Context, agentID string) error
	Publish(ctx context.Context, agentID string) (AgentVersion, error)
	Offline(ctx context.Context, agentID string) error
}
