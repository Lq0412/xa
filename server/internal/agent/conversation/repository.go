package conversation

import "context"

// Repository 隔离会话、轮次和消息的持久化。
type Repository interface {
	Find(ctx context.Context, conversationID string) (Conversation, error)
	List(ctx context.Context, userID string) ([]Conversation, error)
	Save(ctx context.Context, conversation Conversation) error
	AppendMessage(ctx context.Context, message Message) error
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
}
