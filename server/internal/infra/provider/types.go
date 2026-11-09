package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolCall 表示模型完成聚合后的工具调用。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSpec 描述发送给模型的工具及其 JSON Schema。
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolChoiceMode 表示厂商中立的工具选择方式。
type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceFunction ToolChoiceMode = "function"
)

// ToolChoice 控制模型是否以及选择哪个工具。
type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

// ResponseSchema 描述结构化输出约束。
type ResponseSchema struct {
	Name   string
	Schema json.RawMessage
	Strict bool
}

// TraceHints 只用于本地关联日志，不发送给供应商。
type TraceHints struct {
	RunID string
	Step  int
}

// ChatRequest 是 ChatProvider 使用的中立请求。
type ChatRequest struct {
	Model             string
	Messages          []Message
	Tools             []ToolSpec
	ToolChoice        ToolChoice
	ParallelToolCalls *bool
	MaxOutputTokens   int
	ResponseSchema    *ResponseSchema
	Trace             TraceHints
}

// StreamEventType 区分正文、工具增量和用量事件。
type StreamEventType string

const (
	EventTextDelta     StreamEventType = "text_delta"
	EventToolCallStart StreamEventType = "tool_call_start"
	EventToolArgsDelta StreamEventType = "tool_args_delta"
	EventUsage         StreamEventType = "usage"
)

// StreamEvent 是模型流的中立增量事件。
type StreamEvent struct {
	Type      StreamEventType
	Text      string
	Index     int
	ToolName  string
	ArgsDelta string
	Usage     *Usage
}

// FinishReason 是归一化后的结束原因；截断不会被工具调用覆盖。
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishToolCalls     FinishReason = "tool_calls"
	FinishLength        FinishReason = "length"
	FinishContentFilter FinishReason = "content_filter"
	FinishUnknown       FinishReason = "unknown"
)

// Response 是 ChatProvider 返回的完整响应。
type Response struct {
	Message           Message
	FinishReason      FinishReason
	RawFinishReason   string
	Usage             Usage
	Model             string
	ProviderRequestID string
	TTFT              time.Duration
	Duration          time.Duration
	Anomalies         []string
}

// Capabilities 描述模型适配器支持的协议能力。
type Capabilities struct {
	Tools                  bool
	ParallelToolCalls      bool
	ToolChoiceNone         bool
	ToolChoiceFunction     bool
	ReliableFunctionChoice bool
	ToolChoiceRequired     bool
	MaxTools               int
	ContextTokens          int
	MaxInputTokens         int
	OutputReserveTokens    int
}

// TokenLimits 分开配置总上下文、最大输入和输出预留；数值是本地预算【未验证供应商上限】。
type TokenLimits struct {
	ContextTokens       int
	MaxInputTokens      int
	OutputReserveTokens int
}

// ChatProvider 是运行时依赖的模型接口。
type ChatProvider interface {
	Name() string
	Capabilities(model string) Capabilities
	Chat(ctx context.Context, req ChatRequest, sink func(StreamEvent) error) (Response, error)
}

// ProviderErrorKind 供运行时判定重试与终止策略。
type ProviderErrorKind string

const (
	ProviderErrorRateLimited   ProviderErrorKind = "rate_limited"
	ProviderErrorUnavailable   ProviderErrorKind = "unavailable"
	ProviderErrorTimeout       ProviderErrorKind = "timeout"
	ProviderErrorBadRequest    ProviderErrorKind = "bad_request"
	ProviderErrorAuth          ProviderErrorKind = "auth"
	ProviderErrorContentFilter ProviderErrorKind = "content_filter"
	ProviderErrorStreamBroken  ProviderErrorKind = "stream_broken"
	ProviderErrorCanceled      ProviderErrorKind = "canceled"
)

// ProviderError 是适配器对外暴露的脱敏错误。
type ProviderError struct {
	Kind       ProviderErrorKind
	HTTPStatus int
	Code       string
	Message    string
	RetryAfter time.Duration
	Emitted    bool
}

// Error 再次脱敏后裁剪，保护直接构造错误的日志路径；不修改共享错误对象。
func (e *ProviderError) Error() string {
	if e.Message == "" {
		return safeProviderText(string(e.Kind), "")
	}
	return fmt.Sprintf("%s: %s", safeProviderText(string(e.Kind), ""), safeProviderText(e.Message, ""))
}
