//go:build integration

package bootstrap_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
)

func postStreamV2(t *testing.T, h *agenthttp.ConversationHandler, owner auth.Principal, convID, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	// 等待终止事件也要有上限，避免连接问题拖到整个测试包超时。
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", strings.NewReader(body))
	req.Header.Set("X-XA-Stream", "2")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.SetPathValue("conversation_id", convID)
	req = req.WithContext(auth.WithPrincipal(ctx, owner))
	w := httptest.NewRecorder()
	h.Send(w, req)
	return w
}

func firstRunID(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var env map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env); err != nil {
			continue
		}
		if rid, ok := env["run_id"].(string); ok && rid != "" {
			return rid
		}
	}
	t.Fatal("缺少 run_id")
	return ""
}

// TestAcceptance9Cancel 验收 #9：2s 内 run 变为 cancelled，可立即再发。
func TestAcceptance9Cancel(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	rh := &agenthttp.RunHandler{Conversations: chats, Runs: chats.Runs}

	body := `{"content":"不要调用任何工具。从第一行开始写一篇不少于一千二百字的中文长文，分多段输出，每段至少一百字，不要提前结束。"}`
	done := make(chan struct{})
	go func() {
		postStreamV2(t, h, owner, c.ID, body, nil)
		close(done)
	}()
	var runID string
	deadline := time.Now().Add(90 * time.Second)
	cancelled := make(chan struct{})
	go func() {
		for time.Now().Before(deadline) {
			var id, status string
			err := db.QueryRow(context.Background(), `SELECT id, status FROM runs WHERE conversation_id=$1 ORDER BY created_at DESC LIMIT 1`, c.ID).Scan(&id, &status)
			if err == nil && id != "" && status == "running" {
				runID = id
				req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+runID+"/cancel", nil)
				req.SetPathValue("run_id", runID)
				req = req.WithContext(auth.WithPrincipal(context.Background(), owner))
				w := httptest.NewRecorder()
				rh.Cancel(w, req)
				if w.Code != 202 {
					t.Errorf("cancel status=%d", w.Code)
				}
				close(cancelled)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	start := time.Now()
	select {
	case <-cancelled:
	case <-time.After(time.Until(deadline)):
		t.Fatal("未及时发出 cancel")
	}
	var status string
	for time.Since(start) < 2*time.Second {
		_ = db.QueryRow(context.Background(), `SELECT status FROM runs WHERE id=$1`, runID).Scan(&status)
		if status == "cancelled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "cancelled" {
		t.Fatalf("2s 内未 cancelled，当前=%s", status)
	}
	w2 := postStreamV2(t, h, owner, c.ID, `{"content":"请只回复「你好」。这是一条新消息，不需要继续上一条长文。"}`, nil)
	if w2.Code != 200 || !strings.Contains(w2.Body.String(), "event: run.completed") {
		t.Fatalf("取消后无法立即发送: %d %s", w2.Code, w2.Body.String())
	}
	// 创建时的心跳和创建时间相同，真正进入执行后必须续租。
	var renewed bool
	nextID := firstRunID(t, w2.Body.String())
	if err := db.QueryRow(t.Context(), `SELECT heartbeat_at > created_at FROM runs WHERE id=$1`, nextID).Scan(&renewed); err != nil || !renewed {
		t.Fatalf("下一轮没有续租：%v", err)
	}
	<-done
	if runID == "" {
		t.Fatal("缺少 run_id")
	}
}

// TestAcceptance15RedisDegrade 验收 #15：Redis 不可用时 run 仍可完成。
func TestAcceptance15RedisDegrade(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatServiceBadRedis(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	w := postStreamV2(t, h, owner, c.ID, `{"content":"现在几点？如需工具请调用 current_time，一句话总结。"}`, nil)
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var completed bool
	sc := bufio.NewScanner(strings.NewReader(w.Body.String()))
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"type":"run.completed"`) || strings.Contains(sc.Text(), `"run.completed"`) {
			completed = true
		}
	}
	if !completed {
		t.Fatal("未收到 run.completed")
	}
	var n int
	if err := db.QueryRow(context.Background(), `SELECT COUNT(*) FROM run_events WHERE run_id IN (SELECT id FROM runs WHERE conversation_id=$1)`, c.ID).Scan(&n); err != nil || n < 2 {
		t.Fatalf("PG 事件未写入: n=%d err=%v", n, err)
	}
}

// TestAcceptance16Idempotency 验收 #16：同键附着同一 run，消息不重复。
func TestAcceptance16Idempotency(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	key := "idem-" + c.ID
	body := `{"content":"用一句话说 OK"}`
	w1 := postStreamV2(t, h, owner, c.ID, body, map[string]string{"Idempotency-Key": key})
	if w1.Code != 200 {
		t.Fatal(w1.Body.String())
	}
	run1 := firstRunID(t, w1.Body.String())
	w2 := postStreamV2(t, h, owner, c.ID, body, map[string]string{"Idempotency-Key": key})
	if w2.Code != 200 {
		t.Fatal(w2.Body.String())
	}
	run2 := firstRunID(t, w2.Body.String())
	if run1 != run2 {
		t.Fatalf("幂等未附着同一 run: %s vs %s", run1, run2)
	}
	var userCount int
	if err := db.QueryRow(context.Background(), `SELECT COUNT(*) FROM messages WHERE conversation_id=$1 AND role='user'`, c.ID).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Fatalf("user 消息重复: count=%d", userCount)
	}
	bad := postStreamV2(t, h, owner, c.ID, `{"content":"不同内容"}`, map[string]string{"Idempotency-Key": key})
	if bad.Code != 422 {
		t.Fatalf("不同内容应 422 idempotency_key_reused，got %d", bad.Code)
	}
}

// TestAcceptance17ConcurrentSend 验收 #17：并发 POST 一会话只有一个 run。
func TestAcceptance17ConcurrentSend(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	h := &agenthttp.ConversationHandler{Conversations: chats}
	body := `{"content":"并发探针：请先调用 current_time，再用至少五百字描述今天，分段输出。"}`
	ready := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(ready)
		postStreamV2(t, h, owner, c.ID, body, nil)
	}()
	<-ready
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := db.QueryRow(context.Background(), `SELECT status FROM runs WHERE conversation_id=$1 ORDER BY created_at DESC LIMIT 1`, c.ID).Scan(&status)
		if err == nil && status == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	w2 := postStreamV2(t, h, owner, c.ID, `{"content":"第二条应被拒绝"}`, nil)
	if w2.Code != 409 {
		t.Fatalf("第二条期望 409，got %d body=%s", w2.Code, w2.Body.String())
	}
	wg.Wait()
	var runCount int
	_ = db.QueryRow(context.Background(), `SELECT COUNT(*) FROM runs WHERE conversation_id=$1 AND parent_run_id IS NULL`, c.ID).Scan(&runCount)
	if runCount != 1 {
		t.Fatalf("runs=%d", runCount)
	}
}

// TestAcceptance18Sweeper 验收 #18：租约过期后 Sweeper 标记 interrupted。
func TestAcceptance18Sweeper(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	chats := newChatService(t, db, agents, mem)
	list, _ := agents.List(context.Background(), owner)
	c, _ := chats.Create(context.Background(), owner, list[0].ID)
	runID := "run_" + uuid.NewString()
	ctx := context.Background()
	msgID := "msg_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user','zombie','completed',now())`, msgID, c.ID); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,status,user_message_id,provider,model,stream_protocol,limits,toolset,toolset_hash,started_at,lease_owner,lease_expires_at)
SELECT $1,$2,$3,$4,ad.id,av.id,$1,'running',$5,'bailian',av.configuration->>'model',2,'{}','{}','',now(),'dead',now()-interval '2 minutes'
FROM agent_definitions ad JOIN agent_versions av ON av.id=ad.current_version_id
WHERE ad.id=$6 LIMIT 1`, runID, owner.WorkspaceID, owner.UserID, c.ID, msgID, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(ctx, `INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key)
VALUES('tr_zombie',$1,$2,$3,1,0,'c1','memory_write','write','running',$1||':c1')`, runID, c.ID, owner.UserID)
	sw := &run.Sweeper{DB: db, Interval: time.Second, Stale: 1 * time.Second}
	sw.TickOnce(ctx)
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, runID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "interrupted" {
		t.Fatalf("status=%s", status)
	}
	var trStatus string
	_ = db.QueryRow(ctx, `SELECT status FROM tool_results WHERE id='tr_zombie'`).Scan(&trStatus)
	if trStatus != "unknown" {
		t.Fatalf("tool=%s", trStatus)
	}
}
