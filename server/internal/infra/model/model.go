package model

import (
	"context"
	"encoding/json"
)

// Request 是对话模型请求的统一内部表示。
type Request struct {
	Model    string
	Messages []Message
	Tools    []Tool
}

// Message 是模型上下文中的一条消息。
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolCall 保留模型调用标识，使跨通道历史能关联调用与真实工具结果。
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Tool 描述模型可请求的工具。
type Tool struct {
	Name   string
	Schema []byte
}

// Chunk 是模型流式返回的增量。
type Chunk struct {
	Content   string
	ToolCall  []byte
	Completed bool
}

// Client 是对话模型供应商适配边界。
type Client interface {
	Stream(ctx context.Context, request Request) (<-chan Chunk, error)
}
