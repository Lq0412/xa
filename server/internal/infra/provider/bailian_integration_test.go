//go:build integration

package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestBailianFlashTextThenTools 使用真实 flash 和本机时钟完成两次 Chat，验证首段文本不会提前终止工具流。
func TestBailianFlashTextThenTools(t *testing.T) {
	key, base := os.Getenv("XA_BAILIAN_API_KEY"), os.Getenv("XA_BAILIAN_BASE_URL")
	if key == "" || base == "" {
		t.Fatal("缺少百炼环境配置")
	}
	model := NewBailian(key, base)
	messages := []Message{{Role: "system", Content: "在调用任何工具之前，必须先用一句中文向用户说明你要做什么，然后再调用工具。不能只调用工具。"}, {Role: "user", Content: "请先说明你会查询当前时间，然后调用 current_time 查现在时间。"}}
	req := ChatRequest{Model: "qwen3.8-flash", Messages: messages, Tools: []ToolSpec{{Name: "current_time", Description: "查询真实当前时间", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}}}
	var response Response
	matched := false
	for attempt := 0; attempt < 3; attempt++ {
		var emitted strings.Builder
		textBeforeTool := false
		var err error
		response, err = model.Chat(context.Background(), req, func(event StreamEvent) error {
			if event.Type == EventTextDelta {
				emitted.WriteString(event.Text)
			}
			if event.Type == EventToolCallStart && emitted.Len() > 0 {
				textBeforeTool = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if emitted.String() != response.Message.Content {
			t.Fatal("文本下发与聚合不一致")
		}
		if textBeforeTool && len(response.Message.ToolCalls) > 0 {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatal("三次真实调用均未复现先文本后工具，未修改供应商响应")
	}
	if response.FinishReason != FinishToolCalls || response.Usage.TotalTokens == 0 {
		t.Fatal("工具响应状态或用量错误")
	}
	messages = append(messages, response.Message)
	for _, call := range response.Message.ToolCalls {
		if call.Name != "current_time" || call.ID == "" || !json.Valid([]byte(call.Arguments)) {
			t.Fatal("真实工具调用无效")
		}
		// 工具结果来自本机真实时钟，不使用固定返回值。
		result, err := json.Marshal(map[string]string{"time": time.Now().Format(time.RFC3339)})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: string(result)})
	}
	var finalText strings.Builder
	final, err := model.Chat(context.Background(), ChatRequest{Model: req.Model, Messages: messages}, textSink(func(text string) error { finalText.WriteString(text); return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if final.FinishReason != FinishStop || finalText.Len() == 0 || finalText.String() != final.Message.Content || len(final.Message.ToolCalls) > 0 {
		t.Fatal("不带 tools 的真实收尾轮未正常完成")
	}
}
