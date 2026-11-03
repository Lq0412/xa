package http

import "context"

// Handler 暴露 Agent 管理和用户会话 HTTP 接口。
type Handler interface {
	ListAgents(ctx context.Context) error
	CreateAgent(ctx context.Context) error
	PublishAgent(ctx context.Context) error
	CreateConversation(ctx context.Context) error
	SendTurn(ctx context.Context) error
}
