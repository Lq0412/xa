package provider

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLegacyMessageBytes(t *testing.T) {
	// 旧类型保留在测试内，覆盖记忆 prompt 的空串、Unicode、HTML 转义与空切片。
	type oldMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	for _, content := range []string{"", "你好\n世界", `<>&"\\`, "\u2028\u2029"} {
		old, err := json.Marshal([]oldMessage{{Role: "user", Content: content}, {Role: "assistant"}})
		if err != nil {
			t.Fatal(err)
		}
		current, err := json.Marshal([]Message{{Role: "user", Content: content, ToolCalls: []ToolCall{}}, {Role: "assistant"}})
		if err != nil {
			t.Fatal(err)
		}
		if string(current) != string(old) {
			t.Fatal("Message 序列化不再逐字节兼容")
		}
	}
}

func TestBailianRequest(t *testing.T) {
	m := NewBailian("", "http://localhost")
	parallel := true
	req := ChatRequest{Model: "qwen3.8-flash", Messages: []Message{{Role: "assistant", Content: "说明", ToolCalls: []ToolCall{{ID: "call_1", Name: "current_time", Arguments: "{}"}}}, {Role: "tool", Content: "时间", Name: "current_time", ToolCallID: "call_1"}}, ToolChoice: ToolChoice{Mode: ToolChoiceRequired}, ParallelToolCalls: &parallel, Trace: TraceHints{RunID: "must_not_send", Step: 9}}
	data, err := m.encodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tools", "tool_choice", "parallel_tool_calls", "trace", "max_tokens"} {
		if _, ok := body[name]; ok {
			t.Fatalf("未设置工具时不应发送 %s", name)
		}
	}
	if body["enable_thinking"] != false || body["stream_options"].(map[string]any)["include_usage"] != true {
		t.Fatal("流选项错误")
	}
	messages := body["messages"].([]any)
	if messages[0].(map[string]any)["content"] != "" {
		t.Fatal("工具调用历史的 wire content 必须为空")
	}
	if _, ok := messages[1].(map[string]any)["name"]; ok {
		t.Fatal("tool name 不应发送")
	}
	if req.Messages[0].Content != "说明" {
		t.Fatal("不得修改调用方消息")
	}
	req.Tools = []ToolSpec{{Name: "current_time", Parameters: json.RawMessage(`{"type":"object"}`)}}
	for _, choice := range []ToolChoice{{Mode: ToolChoiceAuto}, {Mode: ToolChoiceNone}, {Mode: ToolChoiceFunction, Name: "current_time"}} {
		req.ToolChoice = choice
		if _, err := m.encodeRequest(req); err != nil {
			t.Fatal(err)
		}
	}
	req.ToolChoice.Mode = ToolChoiceRequired
	if _, err := m.encodeRequest(req); err == nil {
		t.Fatal("required 应拒绝")
	}
	req.ToolChoice.Mode = ToolChoiceAuto
	req.ResponseSchema = &ResponseSchema{Name: "schema", Schema: json.RawMessage(`{}`)}
	if _, err := m.encodeRequest(req); err == nil {
		t.Fatal("Tools 与 schema 应互斥")
	}
	req.ResponseSchema = nil
	req.Tools[0].Name = "bad.name"
	if _, err := m.encodeRequest(req); err == nil {
		t.Fatal("非法工具名应拒绝")
	}
}

func TestBailianLegacyStreams(t *testing.T) {
	data, err := os.ReadFile("testdata/bailian/qwen3.8-flash_history_without_tools_param.sse")
	if err != nil {
		t.Fatal(err)
	}
	for _, structured := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body["stream_options"].(map[string]any)["include_usage"] != true {
				t.Error("旧接口未启用 usage")
			}
			_, hasSchema := body["response_format"]
			if hasSchema != structured {
				t.Error("结构化输出设置错误")
			}
			if _, err := w.Write(data); err != nil {
				t.Error(err)
			}
		}))
		m := NewBailian("", server.URL)
		var text strings.Builder
		messages := []Message{{Role: "user", Content: "现在几点？"}}
		var usage Usage
		if structured {
			usage, err = m.StreamStructured(context.Background(), "qwen3.8-flash", messages, json.RawMessage(`{"type":"object"}`), func(s string) error { text.WriteString(s); return nil })
		} else {
			usage, err = m.Stream(context.Background(), "qwen3.8-flash", messages, func(s string) error { text.WriteString(s); return nil })
		}
		server.Close()
		if err != nil || text.Len() == 0 || usage.TotalTokens == 0 {
			t.Fatal("旧流接口回归失败")
		}
	}
}

func TestBailianHTTPErrorMapping(t *testing.T) {
	// 合成 HTTP 错误仅验证映射，不作为百炼实测证据。
	for _, tc := range []struct {
		status int
		code   string
		kind   ProviderErrorKind
	}{
		{429, "", ProviderErrorRateLimited}, {500, "", ProviderErrorUnavailable}, {503, "", ProviderErrorUnavailable},
		{401, "", ProviderErrorAuth}, {403, "", ProviderErrorAuth}, {400, "InvalidParameter", ProviderErrorBadRequest},
		{400, "Throttling", ProviderErrorRateLimited}, {400, "RateLimit", ProviderErrorRateLimited},
		{400, "InvalidApiKey", ProviderErrorAuth}, {400, "DataInspectionFailed", ProviderErrorContentFilter},
	} {
		t.Run(string(tc.kind)+tc.code+http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(tc.status)
				if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": tc.code, "message": strings.Repeat("错误", 200)}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			_, err := NewBailian("", server.URL).Chat(context.Background(), ChatRequest{Model: "qwen", Messages: []Message{{Role: "user", Content: "测试"}}}, nil)
			var problem *ProviderError
			if !errors.As(err, &problem) || problem.Kind != tc.kind || problem.HTTPStatus != tc.status || problem.RetryAfter != 3*time.Second || len([]rune(problem.Message)) != 300 || problem.Emitted {
				t.Fatalf("HTTP 映射错误: %v", err)
			}
		})
	}
	now := time.Now().UTC().Truncate(time.Second)
	if retryAfter(now.Add(time.Minute).Format(http.TimeFormat), now) != time.Minute || retryAfter("-1", now) != 0 {
		t.Fatal("Retry-After 解析错误")
	}
}

func TestBailianCancellationAndTimeout(t *testing.T) {
	for _, mode := range []string{"cancel", "ttfb", "header_only", "call", "sink"} {
		t.Run(mode, func(t *testing.T) {
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Error(err)
					return
				}
				if mode == "call" || mode == "sink" {
					if _, err := io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"开始\"}}]}\n\n"); err != nil {
						return
					}
					w.(http.Flusher).Flush()
				}
				if mode == "header_only" {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			m := NewBailian("", server.URL)
			m.CallTimeout = 80 * time.Millisecond
			m.TTFBTimeout = 30 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			stop := errors.New("接收方停止")
			_, err := m.Chat(ctx, ChatRequest{Model: "qwen", Messages: []Message{{Role: "user", Content: "测试"}}}, func(StreamEvent) error {
				if mode == "sink" {
					return stop
				}
				return nil
			})
			if mode == "sink" {
				if err != stop {
					t.Fatal("必须保留 sink 错误身份")
				}
			} else {
				kind := ProviderErrorTimeout
				if mode == "cancel" {
					kind = ProviderErrorCanceled
				}
				var problem *ProviderError
				if !errors.As(err, &problem) || problem.Kind != kind || problem.Emitted != (mode == "call") {
					t.Fatalf("超时或取消错误: %v", err)
				}
			}
			if mode != "cancel" {
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("未关闭上游请求")
				}
			}
		})
	}
}

func TestBailianSyntheticStreamEdges(t *testing.T) {
	m := NewBailian("", "")
	for _, tc := range []struct {
		raw     string
		finish  FinishReason
		anomaly string
	}{
		{"stop", FinishStop, ""}, {"tool_calls", FinishStop, "tool_calls_without_calls"}, {"length", FinishLength, ""},
		{"sensitive", FinishContentFilter, ""}, {"data_inspection_failed", FinishContentFilter, ""}, {"other", FinishUnknown, ""},
	} {
		a := streamAssembler{finish: tc.raw}
		resp := a.response()
		if resp.FinishReason != tc.finish || tc.anomaly != "" && !slices.Contains(resp.Anomalies, tc.anomaly) {
			t.Fatal("finish 归一化错误")
		}
	}
	for _, body := range []string{"data: invalid\n\n", "data: " + strings.Repeat("x", 4<<20) + "\n\n", ""} {
		_, err := m.assemble(context.Background(), strings.NewReader(body), nil, time.Now(), false)
		var problem *ProviderError
		if !errors.As(err, &problem) || problem.Kind != ProviderErrorStreamBroken || problem.Emitted {
			t.Fatal("非法流应失败且未输出")
		}
	}
	// 合成乱序 index、缺省 index、null id 和交错参数，验证只按 index 聚合而非 ID。
	body := `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"second","arguments":"{"}},{"id":"a","function":{"name":"first","arguments":"{"}}]}}]}` + "\n\n" + `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":null,"function":{"arguments":"}"}},{"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	r, err := m.assemble(context.Background(), strings.NewReader(body), nil, time.Now(), false)
	if err != nil || !reflect.DeepEqual(r.Message.ToolCalls, []ToolCall{{ID: "b", Name: "second", Arguments: "{}"}, {ID: "a", Name: "first", Arguments: "{}"}}) {
		t.Fatal("交错工具聚合错误")
	}
}

func TestRouter(t *testing.T) {
	p := NewBailian("", "")
	specific := NewBailian("", "")
	r := &Router{ByPrefix: map[string]ChatProvider{"qwen": p, "qwen3.8": specific}}
	if r.selectProvider("qwen3.8-flash") != specific || r.selectProvider("qwen3.7-plus") != p {
		t.Fatal("路由未选择最长前缀")
	}
	if _, err := r.Chat(context.Background(), ChatRequest{Model: "missing"}, nil); err == nil {
		t.Fatal("未知模型应失败")
	}
	r.Default = p
	if r.selectProvider("other") != p || r.Capabilities("qwen").ReliableFunctionChoice {
		t.Fatal("路由或能力错误")
	}
	p.Limits = map[string]TokenLimits{"qwen": {ContextTokens: 100, MaxInputTokens: 70, OutputReserveTokens: 30}}
	if c := r.Capabilities("qwen"); c.ContextTokens != 100 || c.MaxInputTokens != 70 || c.OutputReserveTokens != 30 {
		t.Fatal("预算配置未分开传递")
	}
}

func TestProviderErrorRedaction(t *testing.T) {
	secret := rand.Text()
	m := NewBailian(secret, "")
	err := m.providerError(400, bailianError{Code: secret, Message: strings.Repeat("中", 295) + secret + " trailing"}, true)
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Code, secret) || len([]rune(err.Message)) != 300 {
		t.Fatal("错误脱敏或截断顺序错误")
	}
	if err.Code != "[REDACTED]" || !err.Emitted {
		t.Fatal("错误字段未保留安全状态")
	}
}
