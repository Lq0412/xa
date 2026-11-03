package conversation

import (
	"context"

	"linq-bs/server/internal/agent/runtime"
)

// Service 管理会话生命周期并协调一次 Agent 运行。
type Service interface {
	Create(ctx context.Context, conversation Conversation) (Conversation, error)
	Get(ctx context.Context, conversationID string) (Conversation, error)
	List(ctx context.Context, userID string) ([]Conversation, error)
	SendTurn(ctx context.Context, input runtime.RunInput) (<-chan runtime.Event, error)
}
