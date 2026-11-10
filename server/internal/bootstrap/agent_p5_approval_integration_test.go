//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/system/user"
)

// p5Fixture 通过真实记忆 API、百炼和 PostgreSQL 走生产入口。
type p5Fixture struct {
	chats        *conversation.Service
	mem          *memory.Service
	owner, other auth.Principal
	agent        definition.AgentDefinition
	conv         conversation.Conversation
}

func newP5Fixture(t *testing.T, db *pgxpool.Pool, model string) *p5Fixture {
	t.Helper()
	cfg := loadConfig(t)
	users := user.NewService(user.NewPGRepository(db))
	newUser := func() auth.Principal {
		account, err := users.Register(t.Context(), user.Credentials{Email: strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
		if err != nil {
			t.Fatal(err)
		}
		return auth.Principal{UserID: account.ID, WorkspaceID: account.WorkspaceID, Roles: []string{account.Role}}
	}
	owner, other := newUser(), newUser()
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	mem := &memory.Service{DB: db, Agents: agents, Model: provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)}
	chats := newChatService(t, db, agents, mem)
	chats.Runs.Agents = agents
	chats.Runs.Config = cfg.Run
	list, err := agents.List(t.Context(), owner)
	if err != nil || len(list) == 0 {
		t.Fatal(err)
	}
	a := list[0]
	v, err := agents.GetVersion(t.Context(), owner, a.ID, a.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	v.Configuration.Model = model
	thinking := false
	v.Configuration.EnableThinking = &thinking
	v.Configuration.SoulMD = "按用户要求使用工具。用户给出 memory_id 并要求遗忘时，直接调用 memory_forget，不要再搜索或口头询问。收到审批拒绝或超时后不得重试，用一句中文明确说明未删除。工具成功后简短说明结果。"
	v.Configuration.Tools = []binding.ToolBinding{{Name: "memory_forget", Enabled: true, Approval: "always"}}
	v.Configuration.RuntimeLimits = json.RawMessage(`{"max_model_turns":5,"max_tool_calls":8}`)
	saved, err := agents.Save(t.Context(), owner, definition.SaveInput{AgentID: a.ID, ExpectedVersionID: v.ID, Name: a.Name, Visibility: a.Visibility, Configuration: v.Configuration})
	if err != nil {
		t.Fatal(err)
	}
	a = saved.Definition
	c, err := chats.Create(t.Context(), owner, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &p5Fixture{chats, mem, owner, other, a, c}
}

func (f *p5Fixture) add(t *testing.T, subject string) memory.Item {
	t.Helper()
	m, err := f.mem.Add(t.Context(), f.owner, memory.AddInput{Content: "用户喜欢" + subject, Namespace: "preferences", SubjectKey: "p5." + subject, Importance: 6})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *p5Fixture) start(t *testing.T, m memory.Item, wait bool) (string, *run.Approval) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- f.chats.SendStream(ctx, f.owner, f.conv.ID, fmt.Sprintf("忘掉「%s」这条记忆。直接调用 memory_forget，memory_id=%s，reason=用户要求忘掉。", m.Content, m.ID), conversation.StreamSendOptions{Protocol: 2, WriteV2: func(run.StoredEvent) error { return nil }})
	}()
	var rid, status, aid, callID string
	deadline := time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		err := f.chats.DB.QueryRow(t.Context(), `SELECT r.id,r.status,coalesce(a.id,''),coalesce(a.call_id,'') FROM runs r LEFT JOIN run_approvals a ON a.run_id=r.id AND a.status='pending' WHERE r.conversation_id=$1 AND r.created_at>=$2 ORDER BY r.created_at DESC LIMIT 1`, f.conv.ID, started).Scan(&rid, &status, &aid, &callID)
		if err == nil && (aid != "" || (!wait && status == "completed")) {
			break
		}
		select {
		case err := <-done:
			if !wait && err == nil {
				cancel()
				return rid, nil
			}
			cancel()
			t.Fatalf("未等待审批 run=%s status=%s err=%v", rid, status, err)
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	// Redis XREAD 最长阻塞 15 秒，等待订阅结束后才结束测试上下文。
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("SSE 未断开")
	}
	if wait && (status != "waiting_approval" || aid == "") {
		t.Fatalf("未挂起 %s %s", status, aid)
	}
	if !wait {
		if status != "completed" || aid != "" {
			t.Fatalf("授予后仍询问 %s %s", status, aid)
		}
		return rid, nil
	}
	for f.chats.Runs.Coord.Bus(rid) != nil {
		time.Sleep(10 * time.Millisecond)
	}
	a, err := f.chats.Runs.Store.ApprovalForCall(t.Context(), rid, callID)
	if err != nil || a == nil {
		t.Fatal(err)
	}
	return rid, a
}

func (f *p5Fixture) completed(t *testing.T, rid string) {
	t.Helper()
	deadline := time.Now().Add(100 * time.Second)
	var status, text string
	for time.Now().Before(deadline) {
		if err := f.chats.DB.QueryRow(t.Context(), `SELECT r.status,coalesce(m.content,'') FROM runs r LEFT JOIN messages m ON m.id=r.final_message_id WHERE r.id=$1`, rid).Scan(&status, &text); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			if text == "" {
				t.Fatal("空回复")
			}
			break
		}
		if status != "running" {
			t.Fatalf("恢复失败 %s", status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "completed" {
		t.Fatal("恢复未完成", status)
	}
	for f.chats.Runs.Coord.Bus(rid) != nil {
		time.Sleep(10 * time.Millisecond)
	}
	events, err := f.chats.Runs.Store.ListRunEventsAfter(t.Context(), rid, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Fatalf("事件序号不连续 %d %d", i, e.Seq)
		}
	}
}

func TestP5ApprovalLifecycle(t *testing.T) {
	db := newP5Database(t)
	for _, decision := range []string{"approve", "reject", "expire", "cancel", "agent", "conversation", "concurrent"} {
		t.Run(decision, func(t *testing.T) {
			model := "qwen3.8-flash"
			if decision == "approve" {
				model = "qwen3.7-plus"
			}
			f := newP5Fixture(t, db, model)
			m := f.add(t, "coffee")
			if decision == "expire" {
				f.chats.Runs.Config.ApprovalTTLSec = 1
			}
			rid, a := f.start(t, m, true)
			if locked, err := f.chats.Runs.Redis.Exists(t.Context(), "xa:conv:"+f.conv.ID+":writer").Result(); err != nil || locked != 0 {
				t.Fatal("等待未释放锁", locked, err)
			}
			page, err := f.chats.Messages(t.Context(), f.owner, f.conv.ID, "", 20)
			if err != nil || page.ActiveRun == nil || len(page.ActiveRun.PendingApprovals) != 1 {
				t.Fatal("刷新丢失审批", page, err)
			}
			w := postStreamV2(t, &agenthttp.ConversationHandler{Conversations: f.chats}, f.owner, f.conv.ID, `{"content":"等待时发送"}`, nil)
			if w.Code != 409 || !strings.Contains(w.Body.String(), "run_waiting_approval") {
				t.Fatal(w.Code, w.Body.String())
			}
			if decision == "approve" {
				// SSE 已经断开。等待超过默认宽限，再核对 run 仍然可以被批准。
				time.Sleep(31 * time.Second)
				stored, err := f.chats.Runs.Store.GetRun(t.Context(), rid)
				if err != nil || stored.Status != "waiting_approval" || stored.FinalMessageID != "" {
					t.Fatal("断开 SSE 后等待状态丢失", stored.Status, err)
				}
			}
			if decision == "reject" {
				// 后台抽取在审批前实际执行，不能把遗忘请求变成一次绕过审批的删除。
				var source string
				if err := db.QueryRow(t.Context(), `SELECT user_message_id FROM runs WHERE id=$1`, rid).Scan(&source); err != nil {
					t.Fatal(err)
				}
				for {
					var done bool
					if err := db.QueryRow(t.Context(), `SELECT status='completed' FROM memory_tasks WHERE source_ref=$1`, source).Scan(&done); err != nil {
						t.Fatal(err)
					}
					if done {
						break
					}
					worked, err := f.mem.ProcessNext(t.Context())
					if err != nil || !worked {
						t.Fatal("后台抽取未完成", err)
					}
				}
				var status string
				if err := db.QueryRow(t.Context(), `SELECT status FROM memory_items WHERE id=$1`, m.ID).Scan(&status); err != nil || status != "active" {
					t.Fatal("后台抽取绕过审批", status, err)
				}
			}
			_, err = f.chats.Runs.Decide(t.Context(), f.other, a.ID, run.DecisionInput{Decision: "approve"})
			if !errors.Is(err, definition.ErrNotFound) {
				t.Fatal("越权未隐藏审批", err)
			}
			input := run.DecisionInput{Decision: "approve", Remember: "once"}
			if decision == "reject" {
				input.Decision = "reject"
				input.Reason = "暂时保留"
			}
			if decision == "agent" || decision == "conversation" {
				input.Remember = decision
			}
			if decision == "cancel" {
				h := &agenthttp.RunHandler{Runs: f.chats.Runs}
				request := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+rid+"/cancel", nil)
				request.SetPathValue("run_id", rid)
				w := httptest.NewRecorder()
				h.Cancel(w, request.WithContext(auth.WithPrincipal(t.Context(), f.owner)))
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
					t.Fatal(w.Code, w.Body.String())
				}
				after, err := f.chats.Runs.Store.ApprovalForCall(t.Context(), rid, a.CallID)
				if err != nil || after == nil || after.Status != "cancelled" {
					t.Fatal(after, err)
				}
				var stopped string
				if err := db.QueryRow(t.Context(), `SELECT m.error FROM runs r JOIN messages m ON m.id=r.final_message_id WHERE r.id=$1`, rid).Scan(&stopped); err != nil || stopped != "已停止生成" {
					t.Fatal("未保留停止状态", stopped, err)
				}
			} else {
				if decision == "expire" {
					time.Sleep(1100 * time.Millisecond)
					if err := f.chats.Runs.ExpireApprovals(t.Context()); err != nil {
						t.Fatal(err)
					}
				} else if decision == "concurrent" {
					var wg sync.WaitGroup
					results := make(chan error, 2)
					for _, d := range []string{"approve", "reject"} {
						wg.Add(1)
						go func(d string) {
							defer wg.Done()
							_, err := f.chats.Runs.Decide(t.Context(), f.owner, a.ID, run.DecisionInput{Decision: d})
							results <- err
						}(d)
					}
					wg.Wait()
					close(results)
					success, conflict := 0, 0
					for err := range results {
						if err == nil {
							success++
						} else {
							var ae *run.ApprovalError
							if !errors.As(err, &ae) || ae.Status != 409 {
								t.Fatal(err)
							}
							conflict++
						}
					}
					if success != 1 || conflict != 1 {
						t.Fatal(success, conflict)
					}
				} else {
					// 用全新的服务对象决定并恢复，不依赖挂起时的 Coordinator 或 EventBus。
					old := f.chats.Runs
					f.chats.Runs = &run.RunService{DB: old.DB, Store: run.NewPGStore(old.DB), Registry: old.Registry, Provider: old.Provider, Redis: old.Redis, RedisOK: old.RedisOK, Agents: f.chats.Agents, Coord: run.NewCoordinator(), InstanceID: "p5-restarted"}
					if _, err := f.chats.Runs.Decide(t.Context(), f.owner, a.ID, input); err != nil {
						t.Fatal(err)
					}
					if _, err := f.chats.Runs.Decide(t.Context(), f.owner, a.ID, input); err != nil {
						t.Fatal("幂等失败", err)
					}
					conflict := input
					if input.Decision == "approve" {
						conflict.Decision = "reject"
					} else {
						conflict.Decision = "approve"
					}
					if _, err := f.chats.Runs.Decide(t.Context(), f.owner, a.ID, conflict); err == nil {
						t.Fatal("冲突决定成功")
					}
				}
				f.completed(t, rid)
				f.replayApproval(t, rid)
			}
			var memoryStatus, approvalStatus string
			if err := f.chats.DB.QueryRow(t.Context(), `SELECT status FROM memory_items WHERE id=$1`, m.ID).Scan(&memoryStatus); err != nil {
				t.Fatal(err)
			}
			if err := f.chats.DB.QueryRow(t.Context(), `SELECT status FROM run_approvals WHERE id=$1`, a.ID).Scan(&approvalStatus); err != nil {
				t.Fatal(err)
			}
			if (approvalStatus == "approved") != (memoryStatus == "deleted") {
				t.Fatal("决定和副作用不一致", approvalStatus, memoryStatus)
			}
			var count int
			if err := f.chats.DB.QueryRow(t.Context(), `SELECT count(*) FROM memory_events WHERE memory_id=$1 AND op='DELETE'`, m.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if approvalStatus == "approved" {
				wantCount = 1
			}
			if count != wantCount {
				t.Fatal("实际删除次数与决定不符", count, wantCount)
			}
			if decision == "approve" {
				f.checkAdminRecord(t, a.ID, rid)
				if _, err := f.mem.Restore(t.Context(), f.owner, m.ID); err != nil {
					t.Fatal(err)
				}
			}
			if decision == "reject" || decision == "expire" {
				row, err := f.chats.Runs.Store.GetToolResultByCallID(t.Context(), rid, a.CallID)
				want := "approval_rejected"
				if decision == "expire" {
					want = "approval_expired"
				}
				if err != nil || row.ErrorCode != want || !strings.Contains(row.ModelContent, want) {
					t.Fatal(row, err)
				}
			}
			if decision == "conversation" {
				m2 := f.add(t, "tea")
				f.start(t, m2, false)
				// v1 即使有会话授予，也不得把遗忘工具放入快照或执行。
				m3 := f.add(t, "milk")
				h := &agenthttp.ConversationHandler{Conversations: f.chats}
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"content":"请删除 memory_id=%s，调用 memory_forget"}`, m3.ID)))
				req.SetPathValue("conversation_id", f.conv.ID)
				req = req.WithContext(auth.WithPrincipal(t.Context(), f.owner))
				w := httptest.NewRecorder()
				h.Send(w, req)
				if w.Code != 200 || strings.Contains(w.Body.String(), "approval.required") {
					t.Fatal("v1 暴露审批事件")
				}
				var status string
				if err := f.chats.DB.QueryRow(t.Context(), `SELECT status FROM memory_items WHERE id=$1`, m3.ID).Scan(&status); err != nil || status != "active" {
					t.Fatal("v1 记忆状态", status, err)
				}
				v, err := f.chats.Agents.GetVersion(t.Context(), f.owner, f.agent.ID, f.agent.CurrentVersionID)
				if err != nil {
					t.Fatal(err)
				}
				another, err := f.chats.Agents.Save(t.Context(), f.owner, definition.SaveInput{Name: "审批会话隔离", Visibility: definition.VisibilityPrivate, Configuration: v.Configuration})
				if err != nil {
					t.Fatal(err)
				}
				f.conv, err = f.chats.Create(t.Context(), f.owner, another.Definition.ID)
				if err != nil {
					t.Fatal(err)
				}
				newRun, _ := f.start(t, m3, true)
				if status, err := f.chats.Runs.Cancel(t.Context(), f.owner, newRun); err != nil || status != "cancelled" {
					t.Fatal(status, err)
				}
			}
			if decision == "agent" {
				current, err := f.chats.Agents.Get(t.Context(), f.owner, f.agent.ID)
				if err != nil || current.CurrentVersionID == f.agent.CurrentVersionID {
					t.Fatal("未创建新版本", err)
				}
				frozen, err := f.chats.Runs.Store.GetRun(t.Context(), rid)
				entry, _ := frozen.Toolset.EntryByName("memory_forget")
				if err != nil || entry.Approval != "always" {
					t.Fatal("当前工具集被改写", err)
				}
			}
		})
	}
}

// newP5Database 隔离真实抽取任务，避免本机 worker 提前领取。
func newP5Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := loadConfig(t)
	admin, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	cfg.Postgres.Database = "p5_test_" + strings.ToLower(rand.Text())
	id := pgx.Identifier{cfg.Postgres.Database}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+id); err != nil {
			t.Error(err)
		}
	})
	db, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err := db.Exec(t.Context(), "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := user.NewPGRepository(db).EnsureAdmin(t.Context(), cfg.Admin.Email, cfg.Admin.Password); err != nil {
		t.Fatal(err)
	}
	return db
}

// replayApproval 验证同一个订阅可以按原序号重放等待、决定和终止事件。
func (f *p5Fixture) replayApproval(t *testing.T, rid string) {
	t.Helper()
	events, err := f.chats.Runs.Store.ListRunEventsAfter(t.Context(), rid, 0)
	if err != nil {
		t.Fatal(err)
	}
	var required int64
	for _, event := range events {
		if event.Type == run.EventApprovalRequired {
			required = event.Seq
			break
		}
	}
	if required == 0 {
		t.Fatal("缺少审批事件")
	}
	var replay []string
	err = run.Subscribe(t.Context(), f.chats.Runs.Store, f.chats.Runs.Redis, f.chats.Runs.RedisOK, rid, run.StreamOptions{AfterSeq: required - 1}, func(event run.StoredEvent) error {
		if event.Seq != required+int64(len(replay)) {
			t.Fatalf("重放序号不连续 %d", event.Seq)
		}
		replay = append(replay, event.Type)
		return nil
	})
	if err != nil || len(replay) < 4 || replay[0] != run.EventApprovalRequired || replay[1] != run.EventRunWaiting || replay[2] != run.EventApprovalResolved || replay[len(replay)-1] != run.EventRunCompleted {
		t.Fatal("审批重放顺序不符", replay, err)
	}
}

func (f *p5Fixture) checkAdminRecord(t *testing.T, id, rid string) {
	t.Helper()
	h := &agenthttp.RunHandler{Runs: f.chats.Runs}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/approvals?status=approved&tool_name=memory_forget", nil)
	w := httptest.NewRecorder()
	h.ApprovalRecords(w, request.WithContext(auth.WithPrincipal(t.Context(), f.owner)))
	if w.Code != 403 {
		t.Fatal("普通用户可以访问审批列表", w.Code)
	}
	cfg := loadConfig(t)
	account, err := user.NewService(user.NewPGRepository(f.chats.DB)).Authenticate(t.Context(), cfg.Admin.Email, cfg.Admin.Password)
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.Principal{UserID: account.ID, WorkspaceID: account.WorkspaceID, Roles: []string{account.Role}}
	w = httptest.NewRecorder()
	h.ApprovalRecords(w, request.WithContext(auth.WithPrincipal(t.Context(), admin)))
	var records struct {
		Approvals []struct {
			ID     string `json:"id"`
			RunID  string `json:"run_id"`
			WaitMS int64  `json:"wait_ms"`
		} `json:"approvals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &records); w.Code != 200 || err != nil {
		t.Fatal("审批列表查询失败", w.Code, err)
	}
	for _, record := range records.Approvals {
		if record.ID == id && record.RunID == rid && record.WaitMS >= 31000 {
			return
		}
	}
	t.Fatal("审批列表缺少本次记录或等待时间")
}
