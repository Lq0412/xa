package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// streamAssembler 只聚合 choice=0；工具参数保留原文，JSON 校验属于运行时。
type streamAssembler struct {
	text      strings.Builder
	calls     map[int]*partialCall
	order     []int
	finish    string
	usage     *Usage
	sawDone   bool
	emitted   bool
	anomalies []string
	model     string
	requestID string
	ttft      time.Duration
}

type partialCall struct {
	id, name string
	args     strings.Builder
}

type bailianChunk struct {
	ID    string        `json:"id"`
	Model string        `json:"model"`
	Error *bailianError `json:"error"`
	Usage *struct {
		Usage
		PromptDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Choices []struct {
		Index  int    `json:"index"`
		Finish string `json:"finish_reason"`
		Delta  struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning_content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (a *streamAssembler) anomaly(value string) {
	if !slices.Contains(a.anomalies, value) {
		a.anomalies = append(a.anomalies, value)
	}
}

func (a *streamAssembler) emit(event StreamEvent, sink func(StreamEvent) error, start time.Time) error {
	if event.Type != EventUsage {
		if a.ttft == 0 {
			a.ttft = time.Since(start)
		}
		if sink != nil {
			a.emitted = true
		}
	}
	if sink != nil {
		return sink(event)
	}
	return nil
}

func (m *BailianModel) assemble(ctx context.Context, reader io.Reader, sink func(StreamEvent) error, start time.Time, thinkingEnabled bool) (Response, error) {
	a := streamAssembler{calls: make(map[int]*partialCall)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return a.response(), transportError(ctx, ctx.Err(), a.emitted, true)
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			a.sawDone = true
			break
		}
		var chunk bailianChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return a.response(), &ProviderError{Kind: ProviderErrorStreamBroken, Message: "模型流 JSON 无效", Emitted: a.emitted}
		}
		if chunk.Error != nil {
			return a.response(), m.providerError(0, *chunk.Error, a.emitted)
		}
		if chunk.Model != "" {
			a.model = safeProviderText(chunk.Model, m.apiKey)
		}
		if chunk.ID != "" && a.requestID == "" {
			a.requestID = safeProviderText(chunk.ID, m.apiKey)
		}
		if chunk.Usage != nil {
			usage := chunk.Usage.Usage
			usage.CachedTokens = chunk.Usage.PromptDetails.CachedTokens
			a.usage = &usage
			// 独立副本避免接收方修改已记录的用量。
			eventUsage := usage
			if err := a.emit(StreamEvent{Type: EventUsage, Usage: &eventUsage}, sink, start); err != nil {
				return a.response(), err
			}
		}
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				a.anomaly("multi_choice")
				continue
			}
			if text := choice.Delta.Reasoning; text != "" {
				if thinkingEnabled {
					if err := a.emit(StreamEvent{Type: EventReasoningDelta, Reasoning: text}, sink, start); err != nil {
						return a.response(), err
					}
				} else {
					a.anomaly("reasoning_present")
				}
			}
			if text := choice.Delta.Content; text != "" {
				a.text.WriteString(text)
				if err := a.emit(StreamEvent{Type: EventTextDelta, Text: text}, sink, start); err != nil {
					return a.response(), err
				}
			}
			for _, call := range choice.Delta.ToolCalls {
				if call.Index < 0 {
					return a.response(), &ProviderError{Kind: ProviderErrorStreamBroken, Message: "工具 index 无效", Emitted: a.emitted}
				}
				p := a.calls[call.Index]
				if p == nil {
					p = &partialCall{}
					a.calls[call.Index] = p
					a.order = append(a.order, call.Index)
				}
				if call.ID != "" {
					if p.id == "" {
						p.id = call.ID
					} else if p.id != call.ID {
						a.anomaly("tool_call_id_changed")
					}
				}
				if call.Function.Name != "" && p.name == "" {
					p.name = call.Function.Name
					if err := a.emit(StreamEvent{Type: EventToolCallStart, Index: call.Index, ToolName: p.name}, sink, start); err != nil {
						return a.response(), err
					}
				}
				if args := call.Function.Arguments; args != "" {
					p.args.WriteString(args)
					if err := a.emit(StreamEvent{Type: EventToolArgsDelta, Index: call.Index, ArgsDelta: args}, sink, start); err != nil {
						return a.response(), err
					}
				}
			}
			if choice.Finish != "" {
				a.finish = safeProviderText(choice.Finish, m.apiKey)
			}
		}
	}
	if ctx.Err() != nil {
		return a.response(), transportError(ctx, ctx.Err(), a.emitted, true)
	}
	if err := scanner.Err(); err != nil {
		return a.response(), transportError(ctx, err, a.emitted, true)
	}
	if !a.sawDone {
		if a.finish == "" && a.usage == nil {
			return a.response(), &ProviderError{Kind: ProviderErrorStreamBroken, Message: "模型流缺少完成标记", Emitted: a.emitted}
		}
		a.anomaly("missing_done")
	}
	return a.response(), nil
}

func (a *streamAssembler) response() Response {
	result := Response{Message: Message{Role: "assistant", Content: a.text.String()}, RawFinishReason: a.finish, Model: a.model, ProviderRequestID: a.requestID, TTFT: a.ttft}
	if a.usage != nil {
		result.Usage = *a.usage
	}
	used := make(map[string]bool)
	// 按 §4.2 的首次出现顺序输出，保证流式展示与完整调用顺序一致。
	for _, index := range a.order {
		p := a.calls[index]
		id := p.id
		if id == "" {
			id = fmt.Sprintf("call_xa_%d", index)
			a.anomaly("tool_call_id_synthesized")
		}
		base := id
		for n := 1; used[id]; n++ {
			id = fmt.Sprintf("%s_dup%d", base, n)
			a.anomaly("tool_call_id_duplicated")
		}
		used[id] = true
		if p.name == "" {
			a.anomaly("tool_call_without_name")
		}
		args := p.args.String()
		if args == "" {
			args = "{}"
		}
		result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{ID: id, Name: p.name, Arguments: args})
	}
	switch {
	case a.finish == "length":
		result.FinishReason = FinishLength
	case len(result.Message.ToolCalls) > 0:
		result.FinishReason = FinishToolCalls
		if a.finish == "stop" {
			a.anomaly("finish_stop_with_tool_calls")
		} else if a.finish != "tool_calls" {
			a.anomaly("finish_other_with_tool_calls")
		}
	case a.finish == "stop" || a.finish == "tool_calls":
		result.FinishReason = FinishStop
		if a.finish == "tool_calls" {
			a.anomaly("tool_calls_without_calls")
		}
	case a.finish == "content_filter" || a.finish == "sensitive" || a.finish == "data_inspection_failed":
		result.FinishReason = FinishContentFilter
	default:
		result.FinishReason = FinishUnknown
	}
	result.Anomalies = slices.Clone(a.anomalies)
	return result
}
