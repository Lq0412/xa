//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestMemoryS1 使用真实消息、PostgreSQL、百炼抽取和向量，覆盖超过历史窗口后的共享召回。
func TestMemoryS1(t *testing.T) {
	cfg := loadConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	// 独立真实数据库同时验证全新迁移，避免本机 worker 抢走验收任务。
	admin, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	databaseName := "memory_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	cfg.Postgres.Database = databaseName
	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err := db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := user.NewService(user.NewPGRepository(db))
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	newUser := func() auth.Principal {
		account, err := users.Register(ctx, user.Credentials{Email: "memory-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupModuleUser(t, db, account.ID) })
		return auth.Principal{UserID: account.ID, WorkspaceID: account.WorkspaceID}
	}
	// Cleanup 在连接关闭前执行，且只清理本次注册的账号。
	p := newUser()
	other := newUser()
	library, err := (&definition.Library{DB: db}).List(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	primary := library[0]
	version, err := agents.GetVersion(ctx, p, primary.ID, primary.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	save := definition.SaveInput{AgentID: primary.ID, Name: primary.Name, Visibility: primary.Visibility, ExpectedVersionID: version.ID, Configuration: version.Configuration}
	save.Configuration.Model = "qwen3.8-flash"
	save.Configuration.SoulMD = "简短回答，不超过一句话。回答用户个人情况时使用记忆中的事实。"
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
	send := func(agentChat, input string) conversation.Message {
		answer, _, err := chats.Send(ctx, p, agentChat, input, func(string) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		return answer
	}
	drain := func() {
		for {
			var pending int
			if err := db.QueryRow(ctx, `SELECT count(*) FROM memory_tasks t JOIN messages m ON m.id=t.source_ref JOIN conversations c ON c.id=m.conversation_id WHERE c.user_id=$1 AND t.status='pending'`, p.UserID).Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending == 0 {
				break
			}
			if _, err := mem.ProcessNext(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	send(chat.ID, "我叫小林，在准备雅思，目标 7 分")
	drain()
	items, err := mem.List(ctx, p, memory.Filter{Scope: "user", Namespace: "profile"})
	if err != nil || len(items) == 0 {
		t.Fatalf("没有共享 profile: %v", err)
	}
	combined := ""
	for _, item := range items {
		combined += item.Content
		if item.SourceRef == nil {
			t.Fatal("缺少来源")
		}
	}
	if !strings.Contains(combined, "雅思") || !strings.Contains(combined, "7") {
		t.Fatalf("抽取未保留考试目标: %s", combined)
	}
	// 真实闲聊超过 20 条消息，不直接插入伪造历史。
	for _, question := range []string{"天空为什么是蓝色？", "雨后为什么有彩虹？", "月亮为什么有圆缺？", "海水为什么是咸的？", "树叶为什么会变黄？", "冰为什么浮在水面？", "企鹅会飞吗？", "猫为什么打呼噜？", "光传播得有多快？", "太阳是什么颜色？", "云是怎么形成的？"} {
		send(chat.ID, question)
	}
	page, err := chats.Messages(ctx, p, chat.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range page.Messages {
		if strings.Contains(message.Content, "小林") {
			t.Fatal("首次个人消息仍在历史窗口")
		}
	}
	answer := send(chat.ID, "我在准备什么？目标几分？")
	if !strings.Contains(answer.Content, "雅思") || !strings.Contains(answer.Content, "7") {
		t.Fatalf("S1 未通过: %s", answer.Content)
	}
	var segments []runtime.Segment
	var raw []byte
	if err := db.QueryRow(ctx, `SELECT context_segments FROM messages WHERE id=$1`, answer.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, segment := range segments {
		if segment.Name == "memory" && segment.EstimatedTokens > 0 {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("记忆分段未记录: %s", raw)
	}
	secondary, err := agents.Save(ctx, p, definition.SaveInput{Name: "记忆验收 Agent", Visibility: definition.VisibilityPrivate, Configuration: definition.Configuration{Model: "qwen3.8-flash", SoulMD: "简短回答用户问题，参考用户记忆。"}})
	if err != nil {
		t.Fatal(err)
	}
	secondChat, err := chats.GetOrCreate(ctx, p, secondary.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	answer = send(secondChat.ID, "我在准备什么考试？")
	if !strings.Contains(answer.Content, "雅思") {
		t.Fatalf("新 Agent 未读取共享记忆: %s", answer.Content)
	}
	send(secondChat.ID, "请记住，我在这个 Agent 中练习的项目代号是银杏。")
	drain()
	private, err := mem.List(ctx, p, memory.Filter{Scope: "agent", AgentID: secondary.Definition.ID})
	if err != nil || len(private) == 0 {
		t.Fatalf("缺少 agent 层条目: %v", err)
	}
	view, err := mem.Core(ctx, p, primary.ID)
	if err != nil || strings.Contains(view.Markdown, "银杏") {
		t.Fatalf("主 Agent 读到其他 Agent 私有记忆: %v", err)
	}
	view, err = mem.Core(ctx, p, secondary.Definition.ID)
	if err != nil || !strings.Contains(view.Markdown, "银杏") {
		t.Fatalf("本 Agent 无法读取私有记忆: %v", err)
	}
	send(secondChat.ID, "我在这个 Agent 中练习的项目代号改为梧桐，不再是银杏。")
	drain()
	updatedPrivate, err := mem.List(ctx, p, memory.Filter{Scope: "agent", AgentID: secondary.Definition.ID})
	if err != nil {
		t.Fatal(err)
	}
	foundUpdate := false
	for _, item := range updatedPrivate {
		if strings.Contains(item.Content, "梧桐") && item.SupersedesID != nil {
			foundUpdate = true
		}
	}
	if !foundUpdate {
		t.Fatal("真实抽取 UPDATE 没有走取代链")
	}
	foreign, err := mem.List(ctx, other, memory.Filter{})
	if err != nil || len(foreign) != 0 {
		t.Fatal("跨用户泄露")
	}
	if _, err := mem.Events(ctx, other, items[0].ID); !errors.Is(err, definition.ErrNotFound) {
		t.Fatalf("跨用户事件泄露: %v", err)
	}
	if err := mem.Delete(ctx, other, items[0].ID); !errors.Is(err, definition.ErrNotFound) {
		t.Fatalf("跨用户删除: %v", err)
	}
	edit := memory.EditInput{Content: "用户目标雅思8分。", Namespace: "profile", Importance: 10, Pinned: true}
	if _, err := mem.Edit(ctx, other, items[0].ID, edit); !errors.Is(err, definition.ErrNotFound) {
		t.Fatalf("跨用户编辑: %v", err)
	}
	edited, err := mem.Edit(ctx, p, items[0].ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Source != "user_edited" || edited.SupersedesID == nil || *edited.SupersedesID != items[0].ID {
		t.Fatal("没有生成取代链")
	}
	if _, err := mem.Edit(ctx, p, items[0].ID, edit); !errors.Is(err, definition.ErrConflict) {
		t.Fatalf("旧条目可以重复取代: %v", err)
	}
	events, err := mem.Events(ctx, p, edited.ID)
	if err != nil || len(events) != 3 {
		t.Fatalf("取代事件不完整: %d %v", len(events), err)
	}
	active, err := mem.List(ctx, p, memory.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	bounded := memory.Render(active, 80)
	if bounded.EstimatedTokens > 80 || !strings.Contains(bounded.Markdown, "8分") {
		t.Fatalf("预算或 pinned 优先级错误: %+v", bounded)
	}
	tiny := memory.Render(active, 1)
	if tiny.Markdown != "" || tiny.EstimatedTokens != 0 {
		t.Fatal("预算不足时仍注入条目")
	}
	if err := mem.Delete(ctx, p, edited.ID); err != nil {
		t.Fatal(err)
	}
	events, err = mem.Events(ctx, p, items[0].ID)
	if err != nil || len(events) != 4 || events[3].Op != "DELETE" {
		t.Fatal("删除事件缺失")
	}
	// 已关闭的真实模块即使没有服务依赖也不能读取数据库或调用模型。
	result, err := (runtime.Builder{Memory: memory.Module{Policy: definition.MemoryPolicy{Enabled: false}}}).Build(ctx, runtime.Input{Current: "你好"})
	if err != nil || len(result.Segments) != 1 {
		t.Fatal("Builder 未跳过关闭的记忆")
	}
	save.ExpectedVersionID = saved.Version.ID
	save.Configuration.MemoryPolicy = json.RawMessage(`{"enabled":false}`)
	disabled, err := agents.Save(ctx, p, save)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Version.ID == saved.Version.ID {
		t.Fatal("开关未生成版本")
	}
	send(chat.ID, "我正在学习摄影，请记住。")
	var taskCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM memory_tasks t JOIN messages m ON m.id=t.source_ref WHERE m.conversation_id=$1 AND m.content='我正在学习摄影，请记住。'`, chat.ID).Scan(&taskCount); err != nil || taskCount != 0 {
		t.Fatal("关闭后仍写入任务")
	}
	view, err = mem.Core(ctx, p, primary.ID)
	if err != nil || view.Markdown != "" {
		t.Fatal("关闭后仍读取核心记忆")
	}
	// 使用真实 Handler 和数据库验证 HTTP 参数、响应和版本冲突。
	h := &agenthttp.MemoryHandler{Memory: mem, Agents: agents}
	request := func(method, path, id, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(auth.WithPrincipal(ctx, p))
		r.SetPathValue("agent_id", primary.ID)
		r.SetPathValue("memory_id", id)
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	for _, check := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/api/v1/memories?scope=user&namespace=profile", h.List},
		{"/api/v1/agents/" + primary.ID + "/memory", h.Core},
		{"/api/v1/memories/" + edited.ID + "/events", h.Events},
	} {
		if w := request("GET", check.path, edited.ID, "", check.handler); w.Code != 200 || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("HTTP %s: %d", check.path, w.Code)
		}
	}
	body, _ := json.Marshal(map[string]any{"expected_version_id": disabled.Version.ID, "enabled": true, "core_budget_tokens": 100, "recall_top_k": 3})
	if w := request("PUT", "/api/v1/agents/"+primary.ID+"/memory-policy", "", string(body), h.Policy); w.Code != 200 {
		t.Fatalf("切换开关 HTTP %d: %s", w.Code, w.Body.String())
	}
	if w := request("PUT", "/api/v1/agents/"+primary.ID+"/memory-policy", "", string(body), h.Policy); w.Code != 409 {
		t.Fatalf("旧版本未返回409: %d", w.Code)
	}
	// 真实拒绝连接验证抽取失败隔离；不启动伪造模型服务。
	completed := send(chat.ID, "我最喜欢的季节是秋天。")
	failedMem := &memory.Service{DB: db, Agents: agents, Model: provider.NewBailian(cfg.Bailian.APIKey, "http://127.0.0.1:1")}
	if processed, err := failedMem.ProcessNext(ctx); !processed || err == nil {
		t.Fatal("模型连接失败没有报告错误")
	}
	var failureCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM memory_events WHERE owner_user_id=$1 AND op='EXTRACT_FAILED'`, p.UserID).Scan(&failureCount); err != nil || failureCount != 1 {
		t.Fatalf("抽取失败未写事件: %v", err)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM messages WHERE id=$1`, completed.ID).Scan(&status); err != nil || status != "completed" {
		t.Fatal("抽取失败影响已完成对话")
	}
	send(secondChat.ID, "我在这个 Agent 中偏好先看例句再看解释。")
	if err := agents.Delete(ctx, p, secondary.Definition.ID); err != nil {
		t.Fatal(err)
	}
	if processed, err := mem.ProcessNext(ctx); !processed || err == nil {
		t.Fatal("已删除 Agent 的任务没有结束并报告错误")
	}
	var pending int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM memory_tasks WHERE status='pending'`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("失效 Agent 阻塞了任务队列")
	}
	t.Log("S1：超过20条真实历史后回答雅思7分，新建Agent共享profile；取代链、隔离、预算和关闭逻辑通过")
}
