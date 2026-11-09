//go:build integration

package bootstrap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/conversation"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
)

func TestV1ProtocolOnlyDeltaDoneError(t *testing.T) {
	db, agents, reg, mem, owner, _ := testToolsEnv(t)
	list, err := agents.List(context.Background(), owner)
	if err != nil || len(list) == 0 {
		t.Fatal(err)
	}
	agent := list[0]
	convSvc := &conversation.Service{DB: db, Agents: agents, Registry: reg, Memory: mem, AllowedModels: []string{"qwen-plus"}}
	c, err := convSvc.Create(context.Background(), owner, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := &agenthttp.ConversationHandler{Conversations: convSvc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+c.ID+"/messages", strings.NewReader(`{"content":"你好"}`))
	req.SetPathValue("conversation_id", c.ID)
	req = req.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	h.Send(w, req)
	body := w.Body.String()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "event: ") {
			ev := strings.TrimPrefix(line, "event: ")
			if ev != "delta" && ev != "done" && ev != "error" {
				t.Fatalf("v1 不应出现 %s", ev)
			}
		}
	}
}

func TestV1ToolsetOmitsApprovalTools(t *testing.T) {
	_, agents, reg, _, owner, _ := testToolsEnv(t)
	list, _ := agents.List(context.Background(), owner)
	v, _ := agents.GetVersion(context.Background(), owner, list[0].ID, list[0].CurrentVersionID)
	ts, err := reg.Resolve(tool.ResolveInput{
		Bindings: v.Configuration.Tools, Principal: owner, CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ts.Entries {
		if e.Spec.Name == "memory_forget" && e.Enabled {
			t.Fatal("forget 在 v1 应不可用")
		}
	}
}

func TestAcceptanceParallelReadonlyTools(t *testing.T) {
	t.Skip("依赖真实百炼，结构用例由 run 包引擎测试覆盖")
}

func TestMultiTurnToolHistoryProjection(t *testing.T) {
	t.Skip("依赖真实百炼多轮，由 run.TestHistoryProjectorPairing 覆盖不变量")
}

func TestEngineIntegrationKickoff(t *testing.T) {
	_, _, reg, _, owner, _ := testToolsEnv(t)
	ts, err := reg.Resolve(tool.ResolveInput{
		Bindings: []binding.ToolBinding{{Name: "current_time", Enabled: true}},
		Principal: owner, CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var enabled int
	for _, e := range ts.Entries {
		if e.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		t.Fatal("toolset 为空")
	}
}
