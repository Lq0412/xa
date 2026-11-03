package conversation

import (
	"context"

	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
)

// SendInput 只接收客户端可提交的内容；版本、记忆和权限由服务端加载。
type SendInput struct {
	ConversationID string
	RequestID      string
	Message        string
}

// Service 检查私人数据归属和 Agent 当前可用性，保持每个会话仅一个运行通道。
type Service interface {
	Create(ctx context.Context, actor auth.Principal, agentID string) (Conversation, error)
	Get(ctx context.Context, actor auth.Principal, conversationID string) (Conversation, error)
	List(ctx context.Context, actor auth.Principal) ([]Conversation, error)
	SendTurn(ctx context.Context, actor auth.Principal, input SendInput) (<-chan runtime.Event, error)
	OpenRealtime(ctx context.Context, actor auth.Principal, conversationID, requestID string) (runtime.RealtimeSession, error)
	Cancel(ctx context.Context, actor auth.Principal, conversationID string) error
	Delete(ctx context.Context, actor auth.Principal, conversationID string) error
}
