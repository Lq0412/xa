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

func TestV2StreamEnvelopeAndToolEvents(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, err := agents.List(context.Background(), owner)
	if err != nil || len(list) == 0 {
		t.Fatal(err)
	}
	c, err := chats.Create(context.Background(), owner, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	h := &agenthttp.ConversationHandler{Conversations: chats}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+c.ID+"/messages", strings.NewReader(`{"content":"现在几点？如果需要工具就调用 current_time"}`))
	req.Header.Set("X-XA-Stream", "2")
	req.SetPathValue("conversation_id", c.ID)
	req = req.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	sc := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var types []string
	var toolStarted, toolCompleted bool
	var sameCall bool
	var callID string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			types = append(types, strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") {
			var env map[string]any
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env)
			typ, _ := env["type"].(string)
			if typ == "tool.started" {
				toolStarted = true
				cid, _ := env["call_id"].(string)
				if callID == "" {
					callID = cid
				} else {
					sameCall = sameCall || callID == cid
				}
			}
			if typ == "tool.completed" {
				toolCompleted = true
				cid, _ := env["call_id"].(string)
				sameCall = callID != "" && callID == cid
			}
		}
	}
	if !toolStarted || !toolCompleted {
		t.Fatalf("缺少工具生命周期事件 types=%v", types)
	}
	if callID != "" && !sameCall {
		t.Fatal("tool.started 与 tool.completed call_id 不一致")
	}
}

func TestV2ReconnectLastEventID(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	bodyReq := `{"content":"用一句话回复，不要工具"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+c.ID+"/messages", strings.NewReader(bodyReq))
	req.Header.Set("X-XA-Stream", "2")
	req.SetPathValue("conversation_id", c.ID)
	req = req.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	h.Send(w, req)
	var runID string
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: ") {
			var env map[string]any
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env)
			if rid, ok := env["run_id"].(string); ok && runID == "" {
				runID = rid
			}
		}
	}
	if runID == "" {
		t.Fatal("缺少 run_id")
	}
	rh := &agenthttp.RunHandler{Conversations: chats}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID+"/events?after=0", nil)
	req2.Header.Set("Last-Event-ID", "1")
	req2.SetPathValue("run_id", runID)
	req2 = req2.WithContext(auth.WithPrincipal(context.Background(), owner))
	w2 := httptest.NewRecorder()
	rh.Events(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("重连失败: %s", w2.Body.String())
	}
}
