//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

func TestV1ProtocolOnlyDeltaDoneError(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+c.ID+"/messages", strings.NewReader(`{"content":"用一句话打招呼，不要调用任何工具"}`))
	req.SetPathValue("conversation_id", c.ID)
	req = req.WithContext(auth.WithPrincipal(context.Background(), owner))
	w := httptest.NewRecorder()
	h.Send(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("缺少 done: %s", body)
	}
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
		MemoryPolicy: parseMemoryPolicyOrDefault(v.Configuration.MemoryPolicy),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ts.Entries {
		if e.Spec.Name == "memory_forget" && e.Enabled {
			t.Fatal("forget 在 v1 应不可用")
		}
	}
	for _, mt := range ts.ModelTools() {
		if mt.Name == "memory_forget" {
			t.Fatal("ModelTools 含 memory_forget")
		}
	}
}

func configureAgentForToolLoop(t *testing.T, ctx context.Context, agents definition.Service, p auth.Principal, agentID, model string) {
	t.Helper()
	cur, err := agents.Get(context.Background(), p, agentID)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := agents.GetVersion(ctx, p, agentID, cur.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ver.Configuration
	cfg.Model = model
	cfg.SoulMD = "你必须通过工具完成事实类任务：写入记忆用 memory_write，查询用 memory_search，查时间用 current_time。禁止编造未调用工具的结果。回复简洁。"
	cfg.Tools = []binding.ToolBinding{
		{Name: "current_time", Enabled: true},
		{Name: "memory_search", Enabled: true},
		{Name: "memory_write", Enabled: true},
	}
	mp := map[string]any{"enabled": true, "history_budget_tokens": 12000, "core_budget_tokens": 4000, "summary_trigger_tokens": 8000, "min_relevance": 0.25, "recall_top_k": 5}
	raw, _ := json.Marshal(mp)
	cfg.MemoryPolicy = raw
	cfg.RuntimeLimits = json.RawMessage(`{"max_parallel_tools":4,"max_tool_calls":16,"max_model_turns":10,"history_tool_runs":2}`)
	if _, err := agents.Save(ctx, p, definition.SaveInput{
		AgentID: agentID, Name: cur.Name, Visibility: cur.Visibility,
		ExpectedVersionID: ver.ID, Configuration: cfg,
	}); err != nil {
		t.Fatal(err)
	}
}

func parseMemoryPolicyOrDefault(raw json.RawMessage) definition.MemoryPolicy {
	p, _ := definition.ParseMemoryPolicy(raw)
	return p
}

func assertRunCompleted(t *testing.T, ctx context.Context, db *pgxpool.Pool, msgID string) string {
	t.Helper()
	var runID, status string
	if err := db.QueryRow(ctx, `
SELECT COALESCE(m.run_id,''), COALESCE(r.status,'')
FROM messages m LEFT JOIN runs r ON r.id=m.run_id
WHERE m.id=$1`, msgID).Scan(&runID, &status); err != nil {
		t.Fatal(err)
	}
	if runID == "" || status != "completed" {
		t.Fatalf("run 未完成: run_id=%s status=%s", runID, status)
	}
	return runID
}

func TestAcceptanceParallelReadonlyTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	list, _ := agents.List(ctx, owner)
	primary := list[0]
	configureAgentForToolLoop(t, ctx, agents, owner, primary.ID, "qwen3.7-plus")
	tagA := "并行验收A-" + strings.ToLower(rand.Text())
	tagB := "并行验收B-" + strings.ToLower(rand.Text())
	seedUserEditedMemory(t, ctx, db, owner, "preference", "parallel_a", tagA)
	seedUserEditedMemory(t, ctx, db, owner, "preference", "parallel_b", tagB)
	chats := newChatService(t, db, agents, mem)
	conv, err := chats.GetOrCreate(ctx, owner, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	prompt := "你必须在同一条回复里并行发起 3 个函数调用：memory_search(\"" + tagA + "\")、memory_search(\"" + tagB + "\")、current_time()。禁止分步、禁止只调用一个。"
	var peaks int
	for attempt := 0; attempt < 5; attempt++ {
		ans, _, err := chats.Send(ctx, owner, conv.ID, prompt, func(string) error { return nil })
		if err != nil {
			continue
		}
		if ans.Status != "completed" {
			continue
		}
		runID := assertRunCompleted(t, ctx, db, ans.ID)
		if err := db.QueryRow(ctx, `
SELECT COUNT(*) FROM tool_results WHERE run_id=$1 AND status='succeeded'`, runID).Scan(&peaks); err != nil {
			t.Fatal(err)
		}
		var parallelStep *int
		if err := db.QueryRow(ctx, `
SELECT MAX(step) FROM tool_results WHERE run_id=$1 AND status='succeeded'`, runID).Scan(&parallelStep); err != nil {
			t.Fatal(err)
		}
		if parallelStep == nil {
			continue
		}
		var sameStep int
		if err := db.QueryRow(ctx, `
SELECT COUNT(*) FROM tool_results WHERE run_id=$1 AND step=$2 AND status='succeeded'`, runID, *parallelStep).Scan(&sameStep); err != nil {
			t.Fatal(err)
		}
		if peaks >= 2 && sameStep >= 2 {
			var order []string
			rows, err := db.Query(ctx, `SELECT tool_name FROM tool_results WHERE run_id=$1 AND step=$2 ORDER BY call_index`, runID, *parallelStep)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var n string
				_ = rows.Scan(&n)
				order = append(order, n)
			}
			rows.Close()
			if len(order) >= 2 {
				return
			}
		}
	}
	t.Fatal("八次尝试均未满足并行只读调用不变量")
}

func TestMultiTurnToolHistoryProjection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	list, _ := agents.List(ctx, owner)
	primary := list[0]
	configureAgentForToolLoop(t, ctx, agents, owner, primary.ID, "qwen3.7-plus")
	chats := newChatService(t, db, agents, mem)
	conv, err := chats.GetOrCreate(ctx, owner, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	sendUntilToolCalls := func(content string, minTools int) (conversation.Message, string) {
		var last conversation.Message
		var lastErr error
		for attempt := 0; attempt < 5; attempt++ {
			m, _, err := chats.Send(ctx, owner, conv.ID, content, func(string) error { return nil })
			last, lastErr = m, err
			if err != nil || m.Status != "completed" {
				continue
			}
			runID := assertRunCompleted(t, ctx, db, m.ID)
			var n int
			if err := db.QueryRow(ctx, `SELECT tool_calls FROM runs WHERE id=$1`, runID).Scan(&n); err != nil {
				continue
			}
			if minTools <= 0 || n >= minTools {
				return m, runID
			}
		}
		t.Fatalf("发送失败或未调用工具: %v status=%s", lastErr, last.Status)
		return last, ""
	}
	_, run1 := sendUntilToolCalls("【必须调用 memory_write】请记住：我的雅思目标是7分。参数：namespace=goal，subject_key=ielts_target，content=雅思目标7分。不要只用文字回答。", 1)
	_, _ = sendUntilToolCalls("我雅思目标是多少？需要时调用 memory_search 查询 goal 或 ielts_target。", 0)
	_, run3 := sendUntilToolCalls("把雅思目标改成7.5。必须调用 memory_write 更新，仍用 namespace goal 和 subject_key ielts_target，content 写「雅思目标7.5」。", 1)
	var ctxMsgs []provider.Message
	var raw []byte
	if err := db.QueryRow(ctx, `SELECT messages FROM run_contexts WHERE run_id=$1`, run3).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &ctxMsgs); err != nil {
		t.Fatal(err)
	}
	hasToolProtocol := false
	for _, m := range ctxMsgs {
		if m.Role == "tool" || len(m.ToolCalls) > 0 {
			hasToolProtocol = true
			break
		}
	}
	if !hasToolProtocol {
		t.Fatal("第 3 轮上下文未包含历史工具协议投影")
	}
	var toolWrites int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM tool_results WHERE run_id IN ($1,$2) AND tool_name='memory_write' AND status='succeeded'`, run1, run3).Scan(&toolWrites); err != nil {
		t.Fatal(err)
	}
	if toolWrites < 1 {
		t.Fatal("应有成功的 memory_write")
	}
}

func TestEngineIntegrationKickoff(t *testing.T) {
	_, _, reg, _, owner, _ := testToolsEnv(t)
	ts, err := reg.Resolve(tool.ResolveInput{
		Bindings:  []binding.ToolBinding{{Name: "current_time", Enabled: true}},
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
