// Package realtime 定义供应商无关的实时连接能力，不包含 Agent 业务规则。
package realtime

import (
	"context"
	"encoding/json"
	"linq-bs/server/internal/infra/model"
)

// AudioFormat 描述协商后的编码参数。
type AudioFormat struct {
	Encoding   string
	SampleRate int
	Channels   int
}

// Request 使用固定模型版本及已确认历史建立一次连接。
type Request struct {
	ModelProfileID string
	Revision       string
	Instructions   string
	History        []model.Message
	Tools          []model.Tool
	InputAudio     AudioFormat
	OutputAudio    AudioFormat
}

// Input 是通道控制输入，供应商适配器负责协议转换。
// PlaybackOffsetMS 用于打断时裁剪尚未播放的回复。
type Input struct {
	Type             string
	Audio            []byte
	Text             string
	ResponseID       string
	PlaybackOffsetMS int64
}

// Event 已归一化供应商事件，运行层再附加平台轮次和顺序信息。
type Event struct {
	Type       string
	ResponseID string
	Data       json.RawMessage
	Error      error
}

// Session 独占一条上游连接；事件结束时必须关闭通道。
type Session interface {
	Send(ctx context.Context, input Input) error
	SubmitToolResult(ctx context.Context, callID string, result json.RawMessage) error
	Events() <-chan Event
	Close(ctx context.Context) error
}

// Provider 按模型配置选择，Open 失败不能切换到其他模型。
type Provider interface {
	Open(ctx context.Context, request Request) (Session, error)
}
