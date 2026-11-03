package http

import "context"

// Server 是 HTTP、SSE 和 WebSocket 传输层的生命周期接口。
type Server interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
