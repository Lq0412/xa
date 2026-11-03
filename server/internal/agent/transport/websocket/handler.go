package websocket

import "context"

// Handler 暴露语音会话的 WebSocket 生命周期。
type Handler interface {
	Serve(ctx context.Context) error
}
