package sse

import (
	"context"

	"linq-bs/server/internal/agent/runtime"
)

// Streamer 把平台事件编码为 SSE 流。
type Streamer interface {
	Stream(ctx context.Context, events <-chan runtime.Event) error
}
