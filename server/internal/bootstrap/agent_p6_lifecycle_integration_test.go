//go:build integration

package bootstrap_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	toolfake "linq-bs/server/internal/tool/fake"
)

// startP6Approval 只替换模型响应，审批、执行与消息快照均使用真实 PostgreSQL。
func startP6Approval(t *testing.T, db *pgxpool.Pool) (*run.RunService, string, run.Span) {
	t.Helper()
	ctx := t.Context()
	reg := tool.NewRegistry()
	if err := toolfake.Register(reg); err != nil {
		t.Fatal(err)
	}
	ts, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}}, CanApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	first := run.ScriptToolCalls("请确认", provider.ToolCall{ID: "approve", Name: "fake_approval", Arguments: `{}`})
	first.Response.Usage = provider.Usage{PromptTokens: 10, CompletionTokens: 4, CachedTokens: 2}
	first.Response.TTFT = 4 * time.Millisecond
	second := run.ScriptResponse("完成")
	second.Response.Usage = provider.Usage{PromptTokens: 15, CompletionTokens: 2, CachedTokens: 1}
	svc := &run.RunService{DB: db, Store: run.NewPGStore(db), Provider: &run.FakeProvider{Script: []run.FakeScriptEntry{first, second}}, Registry: reg, Coord: run.NewCoordinator(), InstanceID: "p6-test", SnapshotParts: conversation.SnapshotRunParts, Config: config.RunConfig{ApprovalTTLSec: 600, LeaseTTLMS: 30000}}
	id := "run_" + uuid.NewString()
	msg := "msg_" + uuid.NewString()
	now := time.Now().UTC()
	limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 8, MaxCallsPerStep: 4, MaxWallTimeMS: 10000}
	if _, err := db.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status) VALUES($1,'p6_c','user','运行追踪','completed')`, msg); err != nil {
		t.Fatal(err)
	}
	r := run.Run{ID: id, WorkspaceID: "default", OwnerUserID: "p6_u1", ConversationID: "p6_c", AgentID: "p6_a", AgentVersionID: "p6_v", RootRunID: id, UserMessageID: msg, Model: "test", Provider: "fake", StreamProtocol: 2, Status: "running", StartedAt: now, Limits: limits, Toolset: ts}
	messages := []provider.Message{{Role: "user", Content: "运行追踪"}}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := run.InsertRunRow(ctx, tx, svc.Store, r, run.RunContextSnapshot{RunID: id, Messages: messages, Segments: json.RawMessage(`[]`)}, "", "", svc.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	input := run.StreamStartInput{RunID: id, StreamProtocol: 2, ChatStartInput: run.ChatStartInput{Principal: auth.Principal{UserID: "p6_u1", WorkspaceID: "default"}, Conversation: run.ConversationRef{ID: "p6_c"}, Agent: definition.AgentDefinition{ID: "p6_a", OwnerUserID: "p6_u1"}, Version: definition.AgentVersion{ID: "p6_v", Configuration: definition.Configuration{Model: "test"}}, UserMessageID: msg, InitialMessages: messages, Toolset: ts, Limits: limits, Segments: []byte(`[]`), ContextStartedAt: now.Add(-time.Millisecond), ContextEndedAt: now, ContextTrace: []run.ContextObservation{{Slot: "current", Segment: runtime.Segment{Name: "current", EstimatedTokens: 2}, StartedAt: now.Add(-time.Millisecond), EndedAt: now}}}}
	if err := svc.DriveRun(ctx, input, run.NewEventBus(id, nil, nil, svc.Store), nil); err != nil {
		t.Fatal(err)
	}
	spans, err := svc.Store.ListSpans(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range spans {
		if span.Kind == "approval" {
			if span.EndedAt != nil {
				t.Fatal("审批提前结束")
			}
			return svc, id, span
		}
	}
	t.Fatal("缺少审批span")
	return nil, "", run.Span{}
}

func waitP6Terminal(t *testing.T, s *run.RunService, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := s.Store.GetRun(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if r.EndedAt != nil && s.Coord.Bus(id) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("轮次没有结束")
}

// renewP6LeaseDuringSweep 让候选扫描看到旧租约，并在回收取得行锁前提交续租。
func renewP6LeaseDuringSweep(t *testing.T, db *pgxpool.Pool, sw *run.Sweeper, id string) {
	t.Helper()
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE runs SET lease_expires_at=now()+interval '2 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sw.TickOnce(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT last_event_seq FROM runs WHERE id=%FOR UPDATE')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("回收未等待续租事务")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("续租提交后回收未退出")
	}
	r, err := sw.Runs.Store.GetRun(ctx, id)
	if err != nil || r.Status != "running" {
		t.Fatalf("已续租轮次不应被中断，状态=%s，错误=%v", r.Status, err)
	}
}

// TestP6TraceLifecycle 覆盖审批恢复同节点、各类终态快照及模型 token 合计。
func TestP6TraceLifecycle(t *testing.T) {
	db := newP5Database(t)
	seedP6Queries(t, db)
	for _, decision := range []string{"approved", "rejected", "expired", "cancelled", "interrupted"} {
		t.Run(decision, func(t *testing.T) {
			toolfake.ResetCounters()
			svc, id, approval := startP6Approval(t, db)
			if decision == "approved" {
				// 丢弃原协调器、模型和存储对象，只从数据库恢复审批后的执行。
				second := run.ScriptResponse("完成")
				second.Response.Usage = provider.Usage{PromptTokens: 15, CompletionTokens: 2, CachedTokens: 1}
				svc = &run.RunService{DB: db, Store: run.NewPGStore(db), Registry: svc.Registry, Provider: &run.FakeProvider{Script: []run.FakeScriptEntry{second}}, Coord: run.NewCoordinator(), InstanceID: "p6-restarted", SnapshotParts: conversation.SnapshotRunParts, Config: svc.Config}
			}
			p := auth.Principal{UserID: "p6_u1", WorkspaceID: "default"}
			var aid string
			if err := db.QueryRow(t.Context(), `SELECT id FROM run_approvals WHERE run_id=$1`, id).Scan(&aid); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(t.Context(), `UPDATE runs SET started_at=started_at-interval '2 seconds' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			switch decision {
			case "cancelled":
				if _, err := svc.Cancel(t.Context(), p, id); err != nil {
					t.Fatal(err)
				}
			case "interrupted":
				if _, err := db.Exec(t.Context(), `UPDATE runs SET status='running',lease_expires_at=now()-interval '2 minutes',last_event_seq=0,prompt_tokens=0,completion_tokens=0,cached_tokens=0 WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				sw := &run.Sweeper{DB: db, Runs: svc, Stale: time.Second}
				renewP6LeaseDuringSweep(t, db, sw, id)
				if _, err := db.Exec(t.Context(), `UPDATE runs SET lease_expires_at=now()-interval '2 minutes' WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				sw.TickOnce(t.Context())
			case "expired":
				if _, err := db.Exec(t.Context(), `UPDATE run_approvals SET expires_at=now()-interval '1 second' WHERE id=$1`, aid); err != nil {
					t.Fatal(err)
				}
				if err := svc.ExpireApprovals(t.Context()); err != nil {
					t.Fatal(err)
				}
			default:
				d := "approve"
				if decision == "rejected" {
					d = "reject"
				}
				if _, err := svc.Decide(t.Context(), p, aid, run.DecisionInput{Decision: d, Remember: "once"}); err != nil {
					t.Fatal(err)
				}
			}
			waitP6Terminal(t, svc, id)
			wantExecutions := int64(0)
			if decision == "approved" {
				wantExecutions = 1
			}
			if toolfake.ApprovalCount() != wantExecutions {
				t.Fatalf("审批副作用次数=%d，期望%d", toolfake.ApprovalCount(), wantExecutions)
			}
			spans, err := svc.Store.ListSpans(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			counts := map[string]int{}
			byID := map[string]run.Span{}
			var rootID, contextID string
			for _, span := range spans {
				counts[span.Kind]++
				byID[span.ID] = span
				if span.Kind == "run" {
					rootID = span.ID
				}
				if span.Kind == "context" {
					contextID = span.ID
				}
			}
			wantLLM := 2
			if decision == "cancelled" || decision == "interrupted" {
				wantLLM = 1
			}
			if counts["run"] != 1 || counts["context"] != 1 || counts["context_segment"] != 1 || counts["llm"] != wantLLM || counts["approval"] != 1 || counts["tool"] != 1 {
				t.Fatalf("执行树节点数量不完整: %v", counts)
			}
			for _, span := range spans {
				switch span.Kind {
				case "run":
					if span.ParentSpanID != "" {
						t.Fatal("根不能有父节点")
					}
				case "context_segment":
					if span.ParentSpanID != contextID {
						t.Fatal("段未挂在上下文下")
					}
				default:
					if span.ParentSpanID != rootID {
						t.Fatalf("%s未挂在根下", span.Kind)
					}
				}
				if span.ParentSpanID != "" && byID[span.ParentSpanID].StartedAt.After(span.StartedAt) {
					t.Fatal("父节点开始晚于子节点")
				}
			}
			totals := [3]int{}
			rootTokens := [3]int{}
			wantDecision := decision
			if decision == "interrupted" {
				wantDecision = "cancelled"
			}
			for _, s := range spans {
				if s.Kind == "approval" {
					count++
					if s.ID != approval.ID || s.EndedAt == nil || s.Attributes["decision"] != wantDecision {
						t.Fatalf("审批节点未正确恢复与结束: %#v", s)
					}
					if (decision == "approved" || decision == "rejected") && s.Attributes["remember"] != "once" {
						t.Fatal("审批节点缺少记住范围")
					}
				}
				if s.Kind == "llm" && s.PromptTokens != nil {
					totals[0] += *s.PromptTokens
					totals[1] += *s.CompletionTokens
					totals[2] += *s.CachedTokens
				}
				if s.Kind == "run" {
					if s.EndedAt == nil {
						t.Fatal("根未结束")
					}
					rootTokens = [3]int{*s.PromptTokens, *s.CompletionTokens, *s.CachedTokens}
				}
			}
			if count != 1 {
				t.Fatal("重复审批span", count)
			}
			r, err := svc.Store.GetRun(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			want := [3]int{r.PromptTokens, r.CompletionTokens, r.CachedTokens}
			if r.LatencyMS-r.ActiveMS < 1500 {
				t.Fatalf("总耗时未计入等待，耗时=%d，活跃=%d", r.LatencyMS, r.ActiveMS)
			}
			if totals != want || rootTokens != want {
				t.Fatalf("token不一致 llm=%v root=%v run=%v", totals, rootTokens, want)
			}
			var parts []byte
			if err := db.QueryRow(t.Context(), `SELECT parts FROM messages WHERE id=$1`, r.FinalMessageID).Scan(&parts); err != nil {
				t.Fatal(err)
			}
			if len(parts) == 0 || string(parts) == "[]" {
				t.Fatal("缺少终态消息快照")
			}
		})
	}
}
