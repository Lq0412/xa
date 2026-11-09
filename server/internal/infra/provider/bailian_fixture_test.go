package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "更新明确标识的合成 fixtures 和回放 golden")

type fixtureMeta struct {
	Source  string          `json:"source"`
	Model   string          `json:"model"`
	Request json.RawMessage `json:"request_body_without_auth"`
	Notes   string          `json:"notes"`
}

type fixtureGolden struct {
	Response Response
	Error    *ProviderError
	Emitted  bool
}

// replayServer 原样回放已录制的响应，只改变网络分包；不模拟供应商推理行为。
func replayServer(t *testing.T, name string, fragmented bool) (*httptest.Server, ChatRequest) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "bailian", name+".sse"))
	if err != nil {
		t.Fatal(err)
	}
	metaData, err := os.ReadFile(filepath.Join("testdata", "bailian", name+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta fixtureMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Source != "recorded" && meta.Source != "synthetic" {
		t.Fatal("fixture 必须明确标识来源")
	}
	req := ChatRequest{Model: "synthetic", Messages: []Message{{Role: "user", Content: "合成协议回归"}}}
	if meta.Source == "recorded" {
		req = neutralFixtureRequest(t, meta.Request)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
			t.Error("请求路径或方法错误")
		}
		var actual any
		if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
			t.Error(err)
			return
		}
		if meta.Source == "recorded" {
			var expected any
			if err := json.Unmarshal(meta.Request, &expected); err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Error("适配器请求与真实录制请求不同")
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if !fragmented {
			if _, err := w.Write(data); err != nil {
				t.Error(err)
			}
			return
		}
		rng := rand.New(rand.NewPCG(7, 11))
		for remaining := data; len(remaining) > 0; {
			n := min(len(remaining), 1+rng.IntN(97))
			if _, err := w.Write(remaining[:n]); err != nil {
				t.Error(err)
				return
			}
			remaining = remaining[n:]
			if len(data) < 100000 {
				w.(http.Flusher).Flush()
			}
		}
	}))
	t.Cleanup(server.Close)
	return server, req
}

func neutralFixtureRequest(t *testing.T, raw []byte) ChatRequest {
	t.Helper()
	var wire bailianRequest
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	req := ChatRequest{Model: wire.Model, ParallelToolCalls: wire.ParallelToolCalls, MaxOutputTokens: wire.MaxTokens}
	for _, msg := range wire.Messages {
		m := Message{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID}
		for _, c := range msg.ToolCalls {
			m.ToolCalls = append(m.ToolCalls, ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
		}
		req.Messages = append(req.Messages, m)
	}
	for _, tool := range wire.Tools {
		req.Tools = append(req.Tools, tool.Function)
	}
	switch choice := wire.ToolChoice.(type) {
	case string:
		req.ToolChoice.Mode = ToolChoiceMode(choice)
	case map[string]any:
		req.ToolChoice = ToolChoice{Mode: ToolChoiceFunction, Name: choice["function"].(map[string]any)["name"].(string)}
	}
	return req
}

func TestBailianFixtures(t *testing.T) {
	if *updateGolden {
		writeSyntheticFixtures(t)
	}
	paths, err := filepath.Glob("testdata/bailian/*.sse")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 24 {
		t.Fatalf("需要 16 份真实和 8 份合成 fixture，实际 %d", len(paths))
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".sse")
		t.Run(name, func(t *testing.T) {
			for _, fragmented := range []bool{false, true} {
				server, req := replayServer(t, name, fragmented)
				var text strings.Builder
				emitted := false
				resp, err := NewBailian("", server.URL).Chat(context.Background(), req, func(event StreamEvent) error {
					if event.Type != EventUsage {
						emitted = true
					}
					if event.Type == EventTextDelta {
						text.WriteString(event.Text)
					}
					return nil
				})
				if text.String() != resp.Message.Content {
					t.Fatal("增量正文与聚合正文不一致")
				}
				if emitted && resp.TTFT <= 0 || resp.Duration <= 0 {
					t.Fatal("请求时间未记录")
				}
				resp.TTFT, resp.Duration = 0, 0
				actual := fixtureGolden{Response: resp, Emitted: emitted}
				if err != nil && !errors.As(err, &actual.Error) {
					t.Fatalf("错误未归一: %T", err)
				}
				if actual.Error != nil && actual.Error.Emitted != emitted {
					t.Fatal("错误 Emitted 状态不一致")
				}
				goldenPath := strings.TrimSuffix(path, ".sse") + ".golden.json"
				if *updateGolden && !fragmented {
					writeJSON(t, goldenPath, actual)
				}
				data, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatal(err)
				}
				var expected fixtureGolden
				if err := json.Unmarshal(data, &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatal("回放与 golden 不同；仅在核查原始响应后使用 -update")
				}
				assertFixtureContract(t, name, actual)
			}
		})
	}
}

// assertFixtureContract 独立于 golden 生成，锁定设计中的关键不变量和真实异常。
func assertFixtureContract(t *testing.T, name string, g fixtureGolden) {
	t.Helper()
	r := g.Response
	if strings.Contains(name, "specific_") {
		if r.RawFinishReason != "stop" || r.FinishReason != FinishToolCalls || !slices.Contains(r.Anomalies, "finish_stop_with_tool_calls") {
			t.Fatal("未复现指定工具 finish=stop 异常")
		}
		if strings.Contains(name, "parallel") && len(r.Message.ToolCalls) < 2 {
			t.Fatal("未复现额外调用")
		}
	}
	if strings.Contains(name, "text_first_then_tools") && (r.Message.Content == "" || len(r.Message.ToolCalls) < 2) {
		t.Fatal("先文本后工具聚合不完整")
	}
	if strings.Contains(name, "choice_none") && (len(r.Message.ToolCalls) > 0 || !slices.Contains(r.Anomalies, "text_looks_like_tool_call")) {
		t.Fatal("正文 JSON 处理错误")
	}
	if strings.Contains(name, "history_without") && (r.FinishReason != FinishStop || r.Message.Content == "") {
		t.Fatal("收尾轮未完成")
	}
	if strings.HasPrefix(name, "qwen") && (g.Error != nil || r.Usage.TotalTokens == 0 || r.Usage.PromptTokens+r.Usage.CompletionTokens != r.Usage.TotalTokens) {
		t.Fatal("真实回放用量或完成状态错误")
	}
	switch name {
	case "synthetic_broken_mid_args":
		if g.Error == nil || g.Error.Kind != ProviderErrorStreamBroken || !g.Error.Emitted {
			t.Fatal("未检测中途断流")
		}
	case "synthetic_bad_json_args":
		if json.Valid([]byte(r.Message.ToolCalls[0].Arguments)) || r.FinishReason != FinishToolCalls {
			t.Fatal("非法参数不应被适配器修复")
		}
	case "synthetic_length_with_partial_args":
		if r.FinishReason != FinishLength || json.Valid([]byte(r.Message.ToolCalls[0].Arguments)) {
			t.Fatal("截断参数被覆盖")
		}
	case "synthetic_eof_without_done":
		if g.Error != nil || !slices.Contains(r.Anomalies, "missing_done") || r.Usage.CachedTokens != 8 {
			t.Fatal("EOF 完成或缓存用量错误")
		}
	case "synthetic_empty_and_dup_ids":
		ids := make(map[string]bool)
		for _, call := range r.Message.ToolCalls {
			if call.ID == "" || ids[call.ID] {
				t.Fatal("调用 ID 不唯一")
			}
			ids[call.ID] = true
		}
	case "synthetic_stream_error_object":
		if g.Error == nil || g.Error.Kind != ProviderErrorRateLimited || !g.Error.Emitted {
			t.Fatal("流错误映射错误")
		}
	case "synthetic_reasoning_content_present":
		if r.Message.Content != "可见" || !slices.Contains(r.Anomalies, "reasoning_present") || !slices.Contains(r.Anomalies, "multi_choice") {
			t.Fatal("思考或其他 choice 泄漏")
		}
	case "synthetic_huge_line_3mb":
		if g.Error != nil || len(r.Message.Content) != 3<<20 {
			t.Fatal("3 MiB 行读取失败")
		}
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeSyntheticFixtures 显式生成设计要求的异常流；绝不覆写 qwen 开头的实测文件。
func writeSyntheticFixtures(t *testing.T) {
	call := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"memory_search","arguments":"{\"query\":"}}]}}]}` + "\n\n"
	finish := `data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"
	done := "data: [DONE]\n\n"
	fixtures := map[string]string{
		"broken_mid_args":           call,
		"bad_json_args":             call + finish + done,
		"eof_without_done":          `data: {"choices":[{"index":0,"delta":{"content":"完成"},"finish_reason":"stop"}]}` + "\n\n" + `data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":8}}}` + "\n\n",
		"length_with_partial_args":  call + strings.ReplaceAll(finish, `"finish_reason":"tool_calls"`, `"finish_reason":"length"`) + done,
		"empty_and_dup_ids":         `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"current_time"}},{"index":1,"id":"repeat","function":{"name":"current_time","arguments":"{}"}},{"index":2,"id":"repeat","function":{"arguments":"{}"}},{"index":3,"id":"call_xa_0","function":{"name":"current_time"}}]}}]}` + "\n\n" + `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"changed","function":{"arguments":" "}}]}}]}` + "\n\n" + finish + done,
		"stream_error_object":       call + `data: {"error":{"code":"Throttling","message":"限流"}}` + "\n\n",
		"reasoning_content_present": `: keepalive` + "\n\n" + `data: {"choices":[{"index":1,"delta":{"content":"不应下发"}},{"index":0,"delta":{"reasoning_content":"不应下发","content":"可见"},"finish_reason":"stop"}]}` + "\n\n" + done,
		"huge_line_3mb":             `data: {"choices":[{"index":0,"delta":{"content":"` + strings.Repeat("x", 3<<20) + `"},"finish_reason":"stop"}]}` + "\n\n" + done,
	}
	for name, body := range fixtures {
		path := filepath.Join("testdata", "bailian", "synthetic_"+name)
		if err := os.WriteFile(path+".sse", []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, path+".meta.json", fixtureMeta{Source: "synthetic", Model: "synthetic", Notes: "手写合成协议回归；不是百炼实测。场景：" + name})
	}
}

func TestFixtureNoCredentials(t *testing.T) {
	paths, err := filepath.Glob("testdata/bailian/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("sk-")) || bytes.Contains(data, []byte("Bearer")) {
			t.Fatalf("fixture 含凭证标记: %s", filepath.Base(path))
		}
		if key := os.Getenv("XA_BAILIAN_API_KEY"); key != "" && bytes.Contains(data, []byte(key)) {
			t.Fatal("fixture 含环境凭证")
		}
	}
}
