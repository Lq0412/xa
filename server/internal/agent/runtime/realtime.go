package runtime

import (
	"context"
	"linq-bs/server/internal/infra/realtime"
)

// RealtimeInput 使用会话固定版本与已确认上下文，不包含供应商内部音频状态。
type RealtimeInput struct {
	Runtime     RuntimeContext
	Provider    ProviderInput
	Context     ContextSnapshot
	InputAudio  realtime.AudioFormat
	OutputAudio realtime.AudioFormat
}

// RealtimeRunner 创建独立实时连接，不复用单轮文本 Run 接口。
type RealtimeRunner interface {
	Open(ctx context.Context, input RealtimeInput) (RealtimeSession, error)
}

// RealtimeSession 管理音频、打断和播放确认；关闭必须释放会话执行占用。
// Send 的输入来自已认证连接，客户端不能直接提交工具执行结果。
type RealtimeSession interface {
	Send(ctx context.Context, input realtime.Input) error
	Events() <-chan Event
	Close(ctx context.Context) error
}
