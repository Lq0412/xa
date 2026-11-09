//go:build record

package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecordBailianFixtures 直接录制供应商响应，不经过待验证的适配器；只写正文和无凭证元数据。
func TestRecordBailianFixtures(t *testing.T) {
	if os.Getenv("XA_BAILIAN_RECORD") != "1" {
		t.Skip("录制需要 XA_BAILIAN_RECORD=1")
	}
	key, base := os.Getenv("XA_BAILIAN_API_KEY"), os.Getenv("XA_BAILIAN_BASE_URL")
	if key == "" || base == "" {
		t.Fatal("缺少百炼环境配置")
	}
	client := &http.Client{Timeout: 90 * time.Second}
	for _, model := range []string{"qwen3.7-plus", "qwen3.8-flash"} {
		for _, scenario := range recordingScenarios() {
			t.Run(model+"_"+scenario.name, func(t *testing.T) {
				body := scenario.body
				body["model"] = model
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(encoded))
				if err != nil {
					t.Fatal("无法创建录制请求")
				}
				req.Header.Set("Authorization", "Bearer "+key)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal("录制请求失败（不回显连接信息）")
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
				if err != nil {
					t.Fatal("读取录制响应失败")
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("录制 HTTP %d", resp.StatusCode)
				}
				// 在写入任何文件前检查完整正文，绝不记录命中内容。
				check := exec.Command("rg", "-q", "sk-|Bearer")
				check.Stdin = bytes.NewReader(data)
				err = check.Run()
				if bytes.Contains(data, []byte(key)) || err == nil {
					t.Fatal("响应含敏感内容，拒绝落盘")
				}
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatal("响应安全扫描失败")
				}
				path := filepath.Join("testdata", "bailian", model+"_"+scenario.name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".sse", data, 0644); err != nil {
					t.Fatal(err)
				}
				meta, err := json.MarshalIndent(map[string]any{"source": "recorded", "model": model, "request_body_without_auth": body, "recorded_at": time.Now().UTC().Format(time.RFC3339), "notes": "真实百炼原始响应；场景名描述请求意图，实际行为以响应正文和 golden 为准。"}, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".meta.json", append(meta, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
				t.Logf("已录制 %d bytes", len(data))
			})
		}
	}
}

// recordingScenarios 固定请求意图，保留供应商差异；不修改返回的 finish 或工具调用。
func recordingScenarios() []struct {
	name string
	body map[string]any
} {
	tool := func(name, description string, schema json.RawMessage) map[string]any {
		return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": schema}}
	}
	tools := []any{
		tool("memory_search", "搜索用户记忆", json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)),
		tool("current_time", "获取当前时间", json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)),
	}
	var result []struct {
		name string
		body map[string]any
	}
	for _, name := range []string{"auto_parallel_two_tools", "final_answer_after_tool", "choice_none_text_json", "specific_parallel_extra_call_finish_stop", "specific_serial_finish_stop", "text_first_then_tools", "noarg_tool_current_time", "history_without_tools_param"} {
		prompt := "请同时搜索我关于雅思的记忆，并查询现在时间。必须调用 memory_search 和 current_time。"
		body := map[string]any{"stream": true, "stream_options": map[string]bool{"include_usage": true}, "enable_thinking": false, "max_tokens": 1024, "tools": tools, "tool_choice": "auto", "parallel_tool_calls": true}
		system := "你是助手。需要外部信息时使用工具。"
		switch name {
		case "choice_none_text_json":
			body["tool_choice"] = "none"
			prompt = "请直接在正文展示调用 memory_search 查询雅思的 JSON 示例，包含 name 和 arguments，不要解释。"
		case "specific_parallel_extra_call_finish_stop", "specific_serial_finish_stop":
			body["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": "memory_search"}}
			body["parallel_tool_calls"] = name == "specific_parallel_extra_call_finish_stop"
		case "text_first_then_tools":
			system = "在调用任何工具之前，必须先用一句中文向用户说明你要做什么，然后再调用工具。不能只调用工具。"
		case "noarg_tool_current_time":
			prompt = "现在是什么时间？请调用 current_time。"
			body["tools"] = tools[1:]
		}
		messages := []any{map[string]string{"role": "system", "content": system}, map[string]string{"role": "user", "content": prompt}}
		if name == "final_answer_after_tool" || name == "history_without_tools_param" {
			// 历史中的结果是协议示例，不标作真实工具执行；响应本身由真实百炼生成。
			messages = []any{map[string]string{"role": "user", "content": "现在几点？"}, map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call_record_history", "type": "function", "function": map[string]string{"name": "current_time", "arguments": "{}"}}}}, map[string]string{"role": "tool", "tool_call_id": "call_record_history", "content": `{"time":"2026-11-06T09:00:00+08:00"}`}}
			if name == "history_without_tools_param" {
				delete(body, "tools")
				delete(body, "tool_choice")
				delete(body, "parallel_tool_calls")
			}
		}
		body["messages"] = messages
		result = append(result, struct {
			name string
			body map[string]any
		}{name, body})
	}
	return result
}
