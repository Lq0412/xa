package run

import (
	"context"
	"errors"
	"linq-bs/server/internal/infra/provider"
)

// FakeScriptEntry 描述 FakeProvider 的一步响应或错误。
type FakeScriptEntry struct {
	Response *provider.Response
	Err      error
}

// FakeProvider 按脚本顺序返回 Chat 响应，供单元测试使用。
type FakeProvider struct {
	NameVal string
	Script  []FakeScriptEntry
	Index   int
	Caps    provider.Capabilities
}

func (f *FakeProvider) Name() string {
	if f.NameVal != "" {
		return f.NameVal
	}
	return "fake"
}

func (f *FakeProvider) Capabilities(model string) provider.Capabilities {
	if f.Caps.Tools {
		return f.Caps
	}
	return provider.Capabilities{Tools: true, ParallelToolCalls: true, ToolChoiceNone: true, ReliableFunctionChoice: false}
}

func (f *FakeProvider) Chat(ctx context.Context, req provider.ChatRequest, sink func(provider.StreamEvent) error) (provider.Response, error) {
	if f.Index >= len(f.Script) {
		return provider.Response{}, errors.New("FakeProvider 脚本已耗尽")
	}
	entry := f.Script[f.Index]
	f.Index++
	if entry.Err != nil {
		return provider.Response{}, entry.Err
	}
	resp := *entry.Response
	if sink != nil && resp.Message.Content != "" {
		if err := sink(provider.StreamEvent{Type: provider.EventTextDelta, Text: resp.Message.Content}); err != nil {
			return resp, err
		}
	}
	for i, tc := range resp.Message.ToolCalls {
		if sink != nil {
			_ = sink(provider.StreamEvent{Type: provider.EventToolCallStart, Index: i, ToolName: tc.Name})
		}
	}
	return resp, nil
}

// ScriptResponse 构造一条仅文本的响应。
func ScriptResponse(text string) FakeScriptEntry {
	return FakeScriptEntry{Response: &provider.Response{
		Message:      provider.Message{Role: "assistant", Content: text},
		FinishReason: provider.FinishStop,
	}}
}

// ScriptToolCalls 构造带工具调用的响应。
func ScriptToolCalls(text string, calls ...provider.ToolCall) FakeScriptEntry {
	return FakeScriptEntry{Response: &provider.Response{
		Message:      provider.Message{Role: "assistant", Content: text, ToolCalls: calls},
		FinishReason: provider.FinishToolCalls,
	}}
}

// ScriptProviderError 构造可重试的限流错误。
func ScriptProviderError(kind provider.ProviderErrorKind, emitted bool) FakeScriptEntry {
	return FakeScriptEntry{Err: &provider.ProviderError{Kind: kind, Message: "限流", Emitted: emitted}}
}
