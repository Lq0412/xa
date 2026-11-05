//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/runtime"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/system/user"
)

// evalClock 只改变时间；数据库、队列、模型和向量检索均使用真实实现。
type evalClock struct{ at time.Time }

func (c *evalClock) Now() time.Time { return c.at }

// TestMemoryV2 使用独立真实数据库，逐场景输出断言结果与每轮 prompt 预算。
func TestMemoryV2(t *testing.T) {
	if os.Getenv("XA_MEMORY_EVAL") != "1" {
		t.Skip("由 make memory-eval 运行完整长对话")
	}
	cfg := loadConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
	defer cancel()
	admin, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	databaseName := "memory_v2_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, e := admin.Exec(cleanup, "DROP DATABASE "+identifier); e != nil {
			t.Error(e)
		}
	})
	cfg.Postgres.Database = databaseName
	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err = db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	if err = postgres.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var vectorVersion string
	if err = db.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname='vector'`).Scan(&vectorVersion); err != nil {
		t.Fatal(err)
	}
	t.Logf("pgvector=%s", vectorVersion)
	users := user.NewService(user.NewPGRepository(db))
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	register := func() auth.Principal {
		u, e := users.Register(ctx, user.Credentials{Email: "v2-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
		if e != nil {
			t.Fatal(e)
		}
		return auth.Principal{UserID: u.ID, WorkspaceID: u.WorkspaceID}
	}
	p, other := register(), register()
	library, err := (&definition.Library{DB: db}).List(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	primary := library[0]
	version, err := agents.GetVersion(ctx, p, primary.ID, primary.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	policy := definition.MemoryPolicy{Enabled: true, CoreBudgetTokens: 120, RecallTopK: 5, MinRelevance: 0.35, HistoryBudgetTokens: 700, SummaryTriggerTokens: 150, SummaryBudgetTokens: 240, DecayEnabled: true}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	save := definition.SaveInput{AgentID: primary.ID, Name: primary.Name, Visibility: primary.Visibility, ExpectedVersionID: version.ID, Configuration: version.Configuration}
	save.Configuration.Model = "qwen3.8-flash"
	save.Configuration.SoulMD = "用一句简短中文回答，不超过35字。个人事实按记忆回答；不要复述无关个人事实。"
	save.Configuration.MemoryPolicy = raw
	saved, err := agents.Save(ctx, p, save)
	if err != nil {
		t.Fatal(err)
	}
	model := provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)
	mem := &memory.Service{DB: db, Agents: agents, Model: model}
	chats := &conversation.Service{DB: db, Agents: agents, Model: model, Memory: mem, AllowedModels: cfg.ChatModels}
	chat, err := chats.GetOrCreate(ctx, p, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	send := func(id, input string) conversation.Message {
		m, _, e := chats.Send(ctx, p, id, input, func(string) error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		return m
	}
	drain := func() {
		for i := 0; i < 500; i++ {
			processed, e := mem.ProcessNext(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if !processed {
				return
			}
		}
		t.Fatal("队列未清空")
	}
	list := func(f memory.Filter) []memory.Item {
		v, e := mem.List(ctx, p, f)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	request := func(actor auth.Principal, method, path, id, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(auth.WithPrincipal(ctx, actor))
		r.SetPathValue("memory_id", id)
		r.SetPathValue("job_id", id)
		r.SetPathValue("agent_id", primary.ID)
		r.SetPathValue("conversation_id", chat.ID)
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	h := &agenthttp.MemoryHandler{Memory: mem, Agents: agents}
	run := func(name string, fn func(*testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	run("01_long_conversation", func(t *testing.T) {
		questions := []string{"天空为什么是蓝色？", "雨后为什么有彩虹？", "月亮为什么有圆缺？", "海水为什么是咸的？", "树叶为什么会变黄？", "冰为什么浮在水面？", "企鹅会飞吗？", "猫为什么打呼噜？", "光传播得有多快？", "太阳是什么颜色？"}
		for round := 1; round <= 61; round++ {
			input := fmt.Sprintf("自然知识第%d题：%s 请简短说明原因。", round, questions[(round-1)%len(questions)])
			if round == 3 {
				input = "请记住，我的毕业设计项目代号是银杏桥，验收日期是12月18日。"
			}
			if round == 60 || round == 61 {
				input = "我的毕业设计项目代号和验收日期是什么？"
			}
			answer := send(chat.ID, input)
			drain()
			var raw []byte
			if e := db.QueryRow(ctx, `SELECT context_segments FROM messages WHERE id=$1`, answer.ID).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			var segments []runtime.Segment
			if e := json.Unmarshal(raw, &segments); e != nil {
				t.Fatal(e)
			}
			total, bound := 0, policy.HistoryBudgetTokens+policy.SummaryBudgetTokens+2*policy.CoreBudgetTokens
			for _, s := range segments {
				total += s.EstimatedTokens
				if s.Name == "platform" || s.Name == "soul" || s.Name == "current" {
					bound += s.EstimatedTokens
				}
			}
			if total > bound {
				t.Fatalf("round=%d tokens=%d > limit=%d", round, total, bound)
			}
			t.Logf("PROMPT round=%d estimated=%d limit=%d", round, total, bound)
			if round >= 60 && (!strings.Contains(answer.Content, "银杏桥") || !strings.Contains(answer.Content, "18")) {
				t.Fatalf("早期事实未命中: %s", answer.Content)
			}
		}
		var count, versions int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id=$1`, chat.ID).Scan(&count); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(ctx, `SELECT count(*) FROM conversation_summaries WHERE conversation_id=$1`, chat.ID).Scan(&versions); e != nil {
			t.Fatal(e)
		}
		if count < 122 || versions < 2 {
			t.Fatalf("messages=%d summaries=%d", count, versions)
		}
		var injected int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id=$1 AND context_segments @> '[{"name":"session_summary"}]'`, chat.ID).Scan(&injected); e != nil || injected == 0 {
			t.Fatalf("摘要未注入: %v", e)
		}
		t.Logf("RESULT messages=%d summary_versions=%d summary_injected_rounds=%d early_fact_hit=2/2", count, versions, injected)
	})
	run("02_update", func(t *testing.T) {
		send(chat.ID, "请记住，我在准备雅思，目标6.5分。")
		drain()
		send(chat.ID, "我的雅思目标改为7分，不再是6.5分。")
		drain()
		found := false
		for _, m := range list(memory.Filter{}) {
			if strings.Contains(m.Content, "雅思") && strings.Contains(m.Content, "7") && m.SupersedesID != nil {
				found = true
			}
		}
		if !found {
			t.Fatal("没有新目标的取代链")
		}
		answer := send(chat.ID, "我的雅思目标是多少分？")
		drain()
		if !strings.Contains(answer.Content, "7") || strings.Contains(answer.Content, "6.5") {
			t.Fatal(answer.Content)
		}
	})
	run("03_forget", func(t *testing.T) {
		send(chat.ID, "忘掉我在准备雅思，以及我的雅思目标分数。")
		drain()
		deleted := false
		for _, m := range list(memory.Filter{Status: "deleted"}) {
			if strings.Contains(m.Content, "雅思") {
				deleted = true
			}
		}
		if !deleted {
			t.Fatal("未删除雅思事实")
		}
		view, e := mem.Core(ctx, p, primary.ID)
		if e != nil || strings.Contains(view.Markdown, "雅思") {
			t.Fatalf("仍注入已删除事实: %v", e)
		}
	})
	var edited memory.Item
	run("04_user_edited", func(t *testing.T) {
		// 手动保护场景从真实手动写入开始，不依赖早期事实是否由摘要承载。
		var e error
		edited, e = mem.Add(ctx, p, memory.AddInput{Content: "用户毕业设计项目代号是紫藤桥。", Namespace: "profile", SubjectKey: "manual.project", Importance: 9, Pinned: true, Scope: "user"})
		if e != nil {
			t.Fatal(e)
		}
		send(chat.ID, "我的毕业设计项目代号改为桃花桥。")
		drain()
		var status string
		if e = db.QueryRow(ctx, `SELECT status FROM memory_items WHERE id=$1`, edited.ID).Scan(&status); e != nil || status != "active" {
			t.Fatalf("手动记忆被覆盖: %s %v", status, e)
		}
	})
	run("05_decay_restore", func(t *testing.T) {
		send(chat.ID, "我昨天去公园散步了，这是一次普通的日常活动。")
		drain()
		var low memory.Item
		for _, m := range list(memory.Filter{}) {
			if m.Importance <= 5 && !m.Pinned && m.Source != "user_edited" {
				low = m
				break
			}
		}
		if low.ID == "" {
			t.Fatal("模型未抽取低重要度情节记忆")
		}
		clock := &evalClock{at: time.Now().Add(200 * 24 * time.Hour)}
		mem.Clock = clock
		if e := mem.Maintain(ctx); e != nil {
			t.Fatal(e)
		}
		drain()
		var status string
		if e := db.QueryRow(ctx, `SELECT status FROM memory_items WHERE id=$1`, low.ID).Scan(&status); e != nil || status != "archived" {
			t.Fatalf("低价值条目未归档: %s %v", status, e)
		}
		if e := db.QueryRow(ctx, `SELECT status FROM memory_items WHERE id=$1`, edited.ID).Scan(&status); e != nil || status != "active" {
			t.Fatal("pinned 被归档")
		}
		w := request(p, "POST", "/api/v1/memories/"+low.ID+"/restore", low.ID, "", h.Restore)
		if w.Code != 200 {
			t.Fatalf("restore: %d %s", w.Code, w.Body.String())
		}
		var restored memory.Item
		if e := json.Unmarshal(w.Body.Bytes(), &restored); e != nil {
			t.Fatal(e)
		}
		if restored.SupersedesID == nil || *restored.SupersedesID != low.ID {
			t.Fatal("恢复取代链丢失")
		}
		events, e := mem.Events(ctx, p, restored.ID)
		if e != nil {
			t.Fatal(e)
		}
		archive, restore := false, false
		for _, v := range events {
			archive = archive || v.Op == "ARCHIVE"
			restore = restore || v.Op == "RESTORE"
		}
		if !archive || !restore {
			t.Fatal("归档恢复事件缺失")
		}
		mem.Clock = nil
		view, e := mem.Core(ctx, p, primary.ID)
		if e != nil || !strings.Contains(view.Markdown, restored.Content) {
			t.Fatalf("恢复后未重新可见: %v", e)
		}
	})
	run("06_consolidation", func(t *testing.T) {
		send(chat.ID, "我喜欢在清晨沿河慢跑，这个运动习惯请记住。")
		drain()
		var original memory.Item
		for _, m := range list(memory.Filter{}) {
			if strings.Contains(m.Content, "慢跑") && m.Source != "user_edited" {
				original = m
				break
			}
		}
		if original.ID == "" {
			t.Fatal("没有真实抽取的慢跑记忆")
		}
		// 重放真实模型产物模拟升级前重复数据；不伪造向量或模型返回。
		duplicate := "mem_duplicate_" + strings.ToLower(rand.Text())
		if _, e := db.Exec(ctx, `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,agent_id,namespace,subject_key,kind,content,importance,status,source,source_ref,embedding) SELECT $2,workspace_id,owner_user_id,scope,agent_id,namespace,subject_key||'.duplicate',kind,content,importance,'active',source,source_ref,embedding FROM memory_items WHERE id=$1`, original.ID, duplicate); e != nil {
			t.Fatal(e)
		}
		if e := mem.Maintain(ctx); e != nil {
			t.Fatal(e)
		}
		drain()
		var n int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM memory_items WHERE id=ANY($1) AND status='superseded'`, []string{original.ID, duplicate}).Scan(&n); e != nil || n != 2 {
			t.Fatalf("未合并两条: %d %v", n, e)
		}
		events, e := mem.Events(ctx, p, duplicate)
		if e != nil {
			t.Fatal(e)
		}
		merged := false
		for _, v := range events {
			merged = merged || v.Op == "MERGE"
		}
		if !merged {
			t.Fatal("MERGE 事件不可追溯")
		}
	})
	run("07_isolation", func(t *testing.T) {
		secondary, e := agents.Save(ctx, p, definition.SaveInput{Name: "隔离验收", Visibility: definition.VisibilityPrivate, Configuration: definition.Configuration{Model: "qwen3.8-flash", SoulMD: "简短回答。"}})
		if e != nil {
			t.Fatal(e)
		}
		private, e := mem.Add(ctx, p, memory.AddInput{Content: "仅在这个 Agent 使用项目代号青石。", Namespace: "profile", SubjectKey: "private.project", Importance: 8, Scope: "agent", AgentID: secondary.Definition.ID})
		if e != nil {
			t.Fatal(e)
		}
		view, e := mem.Core(ctx, p, primary.ID)
		if e != nil || strings.Contains(view.Markdown, "青石") {
			t.Fatal("Agent 私有层泄露")
		}
		view, e = mem.Core(ctx, p, secondary.Definition.ID)
		if e != nil || !strings.Contains(view.Markdown, "青石") || !strings.Contains(view.Markdown, "紫藤桥") {
			t.Fatal("本 Agent 或 user 共享层不可见")
		}
		for _, actor := range []auth.Principal{other, {UserID: other.UserID, WorkspaceID: other.WorkspaceID, Roles: []string{"admin"}}} {
			w := request(actor, "GET", "/api/v1/memories/"+private.ID+"/events", private.ID, "", h.Events)
			if w.Code != 404 {
				t.Fatalf("跨用户读取: %d", w.Code)
			}
		}
	})
	run("08_retry", func(t *testing.T) {
		send(chat.ID, "请记住，我最喜欢的季节是秋天。")
		var jobID string
		if e := db.QueryRow(ctx, `SELECT id FROM memory_jobs WHERE kind='extract' AND status='pending' ORDER BY created_at DESC LIMIT 1`).Scan(&jobID); e != nil {
			t.Fatal(e)
		}
		// PostgreSQL timestamptz 精度为微秒，与持久化的退避时间使用相同精度。
		clock := &evalClock{at: time.Now().Truncate(time.Microsecond)}
		failed := &memory.Service{DB: db, Agents: agents, Model: provider.NewBailian(cfg.Bailian.APIKey, "http://127.0.0.1:1"), Clock: clock}
		for attempt := 1; attempt <= 3; attempt++ {
			processed, e := failed.ProcessNext(ctx)
			if !processed || e == nil {
				t.Fatal("真实拒绝连接没有失败")
			}
			var attempts int
			var status string
			var next time.Time
			if e = db.QueryRow(ctx, `SELECT attempts,status,next_run_at FROM memory_jobs WHERE id=$1`, jobID).Scan(&attempts, &status, &next); e != nil {
				t.Fatal(e)
			}
			if attempts != attempt {
				t.Fatalf("attempts=%d", attempts)
			}
			want := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}[attempt-1]
			if next.Sub(clock.at) != want {
				t.Fatalf("退避=%s", next.Sub(clock.at))
			}
			if attempt < 3 {
				processed, e = failed.ProcessNext(ctx)
				if processed || e != nil {
					t.Fatal("提前领取未到期任务")
				}
			} else if status != "dead" {
				t.Fatal(status)
			}
			clock.at = next.Add(time.Second)
		}
		if w := request(other, "POST", "/api/v1/memory/jobs/"+jobID+"/retry", jobID, "", h.Retry); w.Code != 404 {
			t.Fatal("跨用户重试")
		}
		if w := request(p, "POST", "/api/v1/memory/jobs/"+jobID+"/retry", jobID, "", h.Retry); w.Code != 204 {
			t.Fatal(w.Body.String())
		}
		drain()
		var status string
		if e := db.QueryRow(ctx, `SELECT status FROM memory_jobs WHERE id=$1`, jobID).Scan(&status); e != nil || status != "completed" {
			t.Fatal("恢复后未成功")
		}
	})
	run("09_threshold", func(t *testing.T) {
		strict := policy
		strict.CoreBudgetTokens = 1
		strict.MinRelevance = 0.9
		details := []runtime.Segment{}
		module := memory.Module{Service: mem, Actor: p, AgentID: primary.ID, Current: "解释量子色动力学的渐近自由", Policy: strict, Details: &details}
		parts, e := module.Context(ctx)
		if e != nil || len(parts) != 0 {
			t.Fatalf("无关问题注入记忆: %v %v", parts, e)
		}
	})
	run("10_api_compatibility", func(t *testing.T) {
		for _, check := range []struct {
			path string
			fn   http.HandlerFunc
		}{{"/api/v1/memories", h.List}, {"/api/v1/agents/" + primary.ID + "/memory", h.Core}, {"/api/v1/memories/" + edited.ID + "/events", h.Events}} {
			w := request(p, "GET", check.path, edited.ID, "", check.fn)
			if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
				t.Fatalf("GET %s: %d", check.path, w.Code)
			}
		}
		body := `{"content":"用户毕业设计项目代号是紫藤桥。","namespace":"profile","importance":9,"pinned":true}`
		w := request(p, "PUT", "/api/v1/memories/"+edited.ID, edited.ID, body, h.Edit)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var next memory.Item
		if e := json.Unmarshal(w.Body.Bytes(), &next); e != nil {
			t.Fatal(e)
		}
		if w = request(p, "DELETE", "/api/v1/memories/"+next.ID, next.ID, "", h.Delete); w.Code != 204 {
			t.Fatal(w.Body.String())
		}
		policyBody := fmt.Sprintf(`{"expected_version_id":%q,"enabled":true,"core_budget_tokens":120,"recall_top_k":5}`, saved.Version.ID)
		if w = request(p, "PUT", "/api/v1/agents/"+primary.ID+"/memory-policy", "", policyBody, h.Policy); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		w = request(p, "POST", "/api/v1/memories", "", `{"content":"用户希望先看结论。","namespace":"preferences","subject_key":"answer.order","kind":"procedure","importance":8,"scope":"user"}`, h.Add)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
		w = request(p, "GET", "/api/v1/memories?limit=1", "", "", h.List)
		var page memory.Page
		if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || page.NextCursor == "" {
			t.Fatal("分页缺少游标")
		}
		w = request(p, "GET", "/api/v1/memories?limit=1&cursor="+page.NextCursor, "", "", h.List)
		var second memory.Page
		if e := json.Unmarshal(w.Body.Bytes(), &second); e != nil || len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
			t.Fatal("游标未前进")
		}
		if w = request(p, "GET", "/api/v1/memories?limit=-1", "", "", h.List); w.Code != 422 {
			t.Fatal("负数 limit 未拒绝")
		}
		if w = request(p, "GET", "/api/v1/memories?scope=all_visible&agent_id="+primary.ID, "", "", h.List); w.Code != 200 {
			t.Fatal("all_visible 失败")
		}
		if w = request(p, "GET", "/api/v1/memory/stats", "", "", h.Stats); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var stats memory.Stats
		if e := json.Unmarshal(w.Body.Bytes(), &stats); e != nil || stats.LatestSummary == nil || stats.LatestMaintain == nil {
			t.Fatal("统计时间缺失")
		}
		summaryHandler := &agenthttp.ConversationHandler{Conversations: chats}
		if w = request(p, "GET", "/api/v1/conversations/"+chat.ID+"/summary", "", "", summaryHandler.Summary); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	})
	t.Log("RESULT scenarios=10/10 early_fact_hit_rate=100%")
}
