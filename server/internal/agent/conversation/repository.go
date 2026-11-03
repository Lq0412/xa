package conversation

import (
	"context"
	"linq-bs/server/internal/framework/auth"
)

// Repository 隔离会话、轮次和消息的持久化。
type Repository interface {
	Find(ctx context.Context, actor auth.Principal, conversationID string) (Conversation, error)
	List(ctx context.Context, actor auth.Principal) ([]Conversation, error)
	Save(ctx context.Context, actor auth.Principal, conversation Conversation) error
	FindTurnByRequest(ctx context.Context, actor auth.Principal, conversationID, requestID string) (Turn, error)
	// BeginTurn 以唯一约束检测重复请求和同会话并发，不重复创建输入消息。
	BeginTurn(ctx context.Context, actor auth.Principal, turn Turn, input Message) error
	FinishTurn(ctx context.Context, actor auth.Principal, turn Turn) error
	AppendMessage(ctx context.Context, actor auth.Principal, message Message) error
	ListMessages(ctx context.Context, actor auth.Principal, conversationID string) ([]Message, error)
}
