//go:build integration

package bootstrap_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
)

func TestMessagesListIncludesToolParts(t *testing.T) {
	db, agents, reg, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	primary := list[0]
	body, _ := json.Marshal(map[string]any{
		"expected_version_id": primary.CurrentVersionID,
		"tools": []map[string]any{
			{"name": "current_time", "enabled": true, "configuration": map[string]any{"timezone": "UTC"}},
		},
		"runtime_limits": map[string]any{"max_tool_calls": 8},
	})
	th := &agenthttp.ToolsHandler{Registry: reg, Agents: agents}
	put := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+primary.ID+"/tools", strings.NewReader(string(body)))
	put.SetPathValue("agent_id", primary.ID)
	put = put.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	th.PutAgentTools(w, put)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c, _ := chats.Create(context.Background(), owner, primary.ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	wStream := postStreamV2(t, h, owner, c.ID, `{"content":"必须先调用 current_time，然后用一句话回答现在几点。"}`, nil)
	if wStream.Code != 200 {
		t.Fatal(wStream.Body.String())
	}
	sc := bufio.NewScanner(strings.NewReader(wStream.Body.String()))
	var hadTool bool
	for sc.Scan() {
		if strings.Contains(sc.Text(), "tool.completed") {
			hadTool = true
		}
	}
	if !hadTool {
		t.Fatal("流式响应缺少 tool.completed")
	}
	page, err := chats.Messages(context.Background(), owner, c.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	var parts []map[string]any
	for _, m := range page.Messages {
		if m.Role == "assistant" {
			for _, p := range m.Parts {
				parts = append(parts, map[string]any(p))
			}
			break
		}
	}
	if len(parts) == 0 {
		t.Fatal("messages 缺少 parts")
	}
	toolOK := false
	for _, p := range parts {
		if p["type"] == "tool" && p["name"] == "current_time" {
			toolOK = true
		}
	}
	if !toolOK {
		t.Fatalf("parts 无 current_time: %#v", parts)
	}
}
