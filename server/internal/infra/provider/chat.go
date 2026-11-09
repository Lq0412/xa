package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Message 是供应商无关的对话消息。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Usage 记录模型返回的 token 用量。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatModel 以增量回调生成回复，回调返回错误时立即停止上游请求。
type ChatModel interface {
	Stream(ctx context.Context, model string, messages []Message, delta func(string) error) (Usage, error)
}

// BailianModel 调用阿里百炼 OpenAI 兼容 chat/completions 接口。
type BailianModel struct {
	apiKey      string
	BaseURL     string
	Client      *http.Client
	CallTimeout time.Duration
	TTFBTimeout time.Duration
	// Limits 是部署方的本地预算配置，不代表已实测的供应商输入限额。
	Limits map[string]TokenLimits
}

// NewBailian 创建关闭思考的适配器，凭证只保存在私有字段中。
func NewBailian(cfgAPIKey, baseURL string) *BailianModel {
	return &BailianModel{apiKey: cfgAPIKey, BaseURL: strings.TrimRight(baseURL, "/"), Client: http.DefaultClient, CallTimeout: 90 * time.Second, TTFBTimeout: 30 * time.Second}
}

// Stream 保持旧文本回调契约，所有协议处理交给 Chat。
func (m *BailianModel) Stream(ctx context.Context, model string, messages []Message, delta func(string) error) (Usage, error) {
	resp, err := m.Chat(ctx, ChatRequest{Model: model, Messages: messages}, textSink(delta))
	return resp.Usage, err
}

// StreamStructured 使用供应商严格 JSON Schema 模式，仍关闭思考。
func (m *BailianModel) StreamStructured(ctx context.Context, model string, messages []Message, schema json.RawMessage, delta func(string) error) (Usage, error) {
	resp, err := m.Chat(ctx, ChatRequest{Model: model, Messages: messages, ResponseSchema: &ResponseSchema{Name: "memory_operations", Strict: true, Schema: schema}}, textSink(delta))
	return resp.Usage, err
}

func textSink(delta func(string) error) func(StreamEvent) error {
	return func(event StreamEvent) error {
		if event.Type == EventTextDelta && delta != nil {
			return delta(event.Text)
		}
		return nil
	}
}
