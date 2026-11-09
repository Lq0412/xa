package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"linq-bs/server/internal/framework/redact"
)

var bailianToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// bailianRequest 是独立 wire 类型，避免改变旧 Message 的 JSON 和记忆 prompt。
type bailianRequest struct {
	Model             string           `json:"model"`
	Messages          []bailianMessage `json:"messages"`
	Tools             []bailianTool    `json:"tools,omitempty"`
	ToolChoice        any              `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool            `json:"parallel_tool_calls,omitempty"`
	Stream            bool             `json:"stream"`
	StreamOptions     map[string]bool  `json:"stream_options"`
	EnableThinking    bool             `json:"enable_thinking"`
	MaxTokens         int              `json:"max_tokens,omitempty"`
	ResponseFormat    any              `json:"response_format,omitempty"`
}

type bailianMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []bailianCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type bailianCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type bailianTool struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

func (m *BailianModel) encodeRequest(req ChatRequest) ([]byte, error) {
	bad := func(message string) ([]byte, error) {
		return nil, &ProviderError{Kind: ProviderErrorBadRequest, Message: message}
	}
	if req.Model == "" || len(req.Messages) == 0 || req.MaxOutputTokens < 0 {
		return bad("模型、消息或输出预算无效")
	}
	if len(req.Tools) > 0 && req.ResponseSchema != nil {
		return bad("Tools 与 ResponseSchema 不能同时设置")
	}
	body := bailianRequest{Model: req.Model, Stream: true, StreamOptions: map[string]bool{"include_usage": true}, MaxTokens: req.MaxOutputTokens}
	for _, message := range req.Messages {
		wire := bailianMessage{Role: message.Role, Content: message.Content}
		switch message.Role {
		case "tool":
			wire.ToolCallID = message.ToolCallID
		case "assistant":
			if len(message.ToolCalls) > 0 {
				wire.Content = ""
			}
			for _, call := range message.ToolCalls {
				if !bailianToolName.MatchString(call.Name) {
					return bad("历史工具名称无效")
				}
				c := bailianCall{ID: call.ID, Type: "function"}
				c.Function.Name, c.Function.Arguments = call.Name, call.Arguments
				wire.ToolCalls = append(wire.ToolCalls, c)
			}
		case "user", "system":
		default:
			return bad("消息角色无效")
		}
		body.Messages = append(body.Messages, wire)
	}
	if len(req.Tools) > 0 {
		if len(req.Tools) > m.Capabilities(req.Model).MaxTools {
			return bad("工具数量超过本地上限")
		}
		for _, spec := range req.Tools {
			if !bailianToolName.MatchString(spec.Name) || !json.Valid(spec.Parameters) {
				return bad("工具名称或参数 Schema 无效")
			}
			body.Tools = append(body.Tools, bailianTool{Type: "function", Function: spec})
		}
		switch req.ToolChoice.Mode {
		case "", ToolChoiceAuto:
			body.ToolChoice = "auto"
		case ToolChoiceNone:
			body.ToolChoice = "none"
		case ToolChoiceRequired:
			return bad("百炼不保证 required 工具选择")
		case ToolChoiceFunction:
			if !bailianToolName.MatchString(req.ToolChoice.Name) {
				return bad("指定工具名称无效")
			}
			found := false
			for _, spec := range req.Tools {
				if spec.Name == req.ToolChoice.Name {
					found = true
				}
			}
			if !found {
				return bad("指定工具未包含在 Tools 中")
			}
			body.ToolChoice = map[string]any{"type": "function", "function": map[string]string{"name": req.ToolChoice.Name}}
		default:
			return bad("工具选择模式无效")
		}
		body.ParallelToolCalls = req.ParallelToolCalls
	}
	if schema := req.ResponseSchema; schema != nil {
		if schema.Name == "" || !json.Valid(schema.Schema) {
			return bad("输出 Schema 无效")
		}
		body.ResponseFormat = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": schema.Name, "strict": schema.Strict, "schema": schema.Schema}}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return bad("无法编码模型请求")
	}
	return encoded, nil
}

// Chat 发送一次流请求；只有成功返回的 Response 可以用于执行工具，错误时保留已聚合的观测结果。
func (m *BailianModel) Chat(ctx context.Context, req ChatRequest, sink func(StreamEvent) error) (result Response, err error) {
	start := time.Now()
	defer func() { result.Duration = time.Since(start) }()
	body, err := m.encodeRequest(req)
	if err != nil {
		return result, err
	}
	if m.Client == nil || m.CallTimeout <= 0 || m.TTFBTimeout <= 0 {
		return result, &ProviderError{Kind: ProviderErrorBadRequest, Message: "模型客户端或超时配置无效"}
	}
	callCtx, cancelTimeout := context.WithTimeout(ctx, m.CallTimeout)
	defer cancelTimeout()
	callCtx, cancel := context.WithCancelCause(callCtx)
	defer cancel(nil)
	// 首字节包括响应正文，不只等待响应头；收到首字节后仍受整次调用期限约束。
	timer := time.AfterFunc(m.TTFBTimeout, func() { cancel(context.DeadlineExceeded) })
	defer timer.Stop()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, m.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return result, &ProviderError{Kind: ProviderErrorBadRequest, Message: "模型地址无效"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := m.Client.Do(httpReq)
	if err != nil {
		return result, transportError(callCtx, err, false, false)
	}
	defer resp.Body.Close()
	reader := &firstByteReader{reader: resp.Body, timer: timer}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(reader, 64<<10))
		if readErr != nil {
			return result, transportError(callCtx, readErr, false, false)
		}
		var envelope struct {
			Error   bailianError `json:"error"`
			Code    string       `json:"code"`
			Message string       `json:"message"`
		}
		parseErr := json.Unmarshal(data, &envelope)
		problem := envelope.Error
		if problem.Code == "" {
			problem.Code = envelope.Code
		}
		if problem.Message == "" {
			problem.Message = envelope.Message
		}
		if parseErr != nil || problem.Message == "" {
			problem.Message = string(data)
		}
		failure := m.providerError(resp.StatusCode, problem, false)
		failure.RetryAfter = retryAfter(resp.Header.Get("Retry-After"), time.Now())
		return result, failure
	}
	result, err = m.assemble(callCtx, reader, sink, start)
	// 仅提供观测标签，不把正文 JSON 转换成可执行调用；后续运行时可写入 span。
	if req.ToolChoice.Mode == ToolChoiceNone && strings.Contains(result.Message.Content, `"name"`) && strings.Contains(result.Message.Content, `"arguments"`) {
		result.Anomalies = append(result.Anomalies, "text_looks_like_tool_call")
	}
	if id := resp.Header.Get("x-request-id"); id != "" {
		result.ProviderRequestID = safeProviderText(id, m.apiKey)
	}
	return result, err
}

type firstByteReader struct {
	reader io.Reader
	timer  *time.Timer
}

func (r *firstByteReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.timer.Stop()
	}
	return n, err
}

type bailianError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func safeProviderText(value, key string) string {
	runes := []rune(redact.String(value, key))
	if len(runes) > 300 {
		runes = runes[:300]
	}
	return string(runes)
}

func (m *BailianModel) providerError(status int, problem bailianError, emitted bool) *ProviderError {
	code := strings.ToLower(problem.Code + " " + problem.Type)
	kind := ProviderErrorStreamBroken
	switch {
	case strings.Contains(code, "datainspection"), strings.Contains(code, "content_filter"), strings.Contains(code, "sensitive"):
		kind = ProviderErrorContentFilter
	case status == 401 || status == 403 || strings.Contains(code, "invalidapikey"):
		kind = ProviderErrorAuth
	case status == 429 || strings.Contains(code, "throttling") || strings.Contains(code, "ratelimit") || strings.Contains(code, "rate_limit"):
		kind = ProviderErrorRateLimited
	case status >= 500 || strings.Contains(code, "serviceunavailable") || strings.Contains(code, "internalerror"):
		kind = ProviderErrorUnavailable
	case status == 400 || strings.Contains(code, "invalidparameter"):
		kind = ProviderErrorBadRequest
	case status >= 300:
		kind = ProviderErrorBadRequest
	}
	return &ProviderError{Kind: kind, HTTPStatus: status, Code: safeProviderText(problem.Code, m.apiKey), Message: safeProviderText(problem.Message, m.apiKey), Emitted: emitted}
}

func transportError(ctx context.Context, err error, emitted, streaming bool) *ProviderError {
	kind := ProviderErrorUnavailable
	var netErr net.Error
	switch {
	case errors.Is(context.Cause(ctx), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		kind = ProviderErrorTimeout
	case ctx.Err() != nil:
		kind = ProviderErrorCanceled
	case errors.As(err, &netErr) && netErr.Timeout():
		kind = ProviderErrorTimeout
	case streaming:
		kind = ProviderErrorStreamBroken
	}
	return &ProviderError{Kind: kind, Emitted: emitted, Message: "模型请求未正常完成"}
}

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}
