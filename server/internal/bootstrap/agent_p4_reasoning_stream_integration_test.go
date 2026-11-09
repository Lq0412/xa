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

func TestV2StreamIncludesReasoningDeltaWhenThinkingEnabled(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	primary := list[0]
	ah := &agenthttp.ManagementHandler{Agents: agents}
	cfgBody := map[string]any{
		"agent_id":            primary.ID,
		"expected_version_id": primary.CurrentVersionID,
		"name":                primary.Name,
		"visibility":          "private",
		"configuration": map[string]any{
			"model":           "qwen3.7-plus",
			"enable_thinking": true,
		},
	}
	raw, _ := json.Marshal(cfgBody)
	put := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+primary.ID+"/config", strings.NewReader(string(raw)))
	put.Header.Set("Content-Type", "application/json")
	put.SetPathValue("agent_id", primary.ID)
	put = put.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	ah.Save(w, put)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var saved struct {
		CurrentVersionID string `json:"current_version_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	primary.CurrentVersionID = saved.CurrentVersionID
	c, _ := chats.Create(context.Background(), owner, primary.ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	wStream := postStreamV2(t, h, owner, c.ID, `{"content":"请先思考再一句话自我介绍，不要调用工具。"}`, nil)
	if wStream.Code != 200 {
		t.Fatal(wStream.Body.String())
	}
	sc := bufio.NewScanner(strings.NewReader(wStream.Body.String()))
	var deltaChars int
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var env map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env); err != nil {
			continue
		}
		if env["type"] != "reasoning.delta" {
			continue
		}
		d, _ := env["delta"].(string)
		deltaChars += len([]rune(d))
	}
	if deltaChars < 8 {
		t.Fatalf("SSE 缺少有效 reasoning.delta 正文，累计=%d", deltaChars)
	}
}
