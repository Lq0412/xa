//go:build integration

package bootstrap_test

import (
	"testing"
	"time"

	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/framework/auth"
)

// TestP6TerminalWriteAtomicity 用真实数据库拒绝终态事件，验证消息、状态和快照不会部分提交。
func TestP6TerminalWriteAtomicity(t *testing.T) {
	db := newP5Database(t)
	seedP6Queries(t, db)
	svc, id, _ := startP6Approval(t, db)
	if _, err := db.Exec(t.Context(), `CREATE FUNCTION reject_p6_terminal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='run.completed' THEN RAISE EXCEPTION 'P6 terminal write rejected'; END IF; RETURN NEW; END $$;
CREATE TRIGGER reject_p6_terminal BEFORE INSERT ON run_events FOR EACH ROW EXECUTE FUNCTION reject_p6_terminal()`); err != nil {
		t.Fatal(err)
	}
	var approvalID string
	if err := db.QueryRow(t.Context(), `SELECT id FROM run_approvals WHERE run_id=$1`, id).Scan(&approvalID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(t.Context(), auth.Principal{UserID: "p6_u1", WorkspaceID: "default"}, approvalID, run.DecisionInput{Decision: "approve", Remember: "once"}); err != nil {
		t.Fatal(err)
	}
	// 第二个模型节点已写入且后台执行已退出，保证断言发生在失败事务之后。
	deadline := time.Now().Add(5 * time.Second)
	finished := false
	for time.Now().Before(deadline) {
		var models int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FROM run_spans WHERE run_id=$1 AND kind='llm'`, id).Scan(&models); err != nil {
			t.Fatal(err)
		}
		if models == 2 && svc.Coord.Bus(id) == nil {
			finished = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !finished {
		t.Fatal("终态写入失败后执行没有退出")
	}
	var status string
	var messages, terminals int
	if err := db.QueryRow(t.Context(), `SELECT status,(SELECT count(*) FROM messages WHERE run_id=$1 AND role='assistant'),(SELECT count(*) FROM run_events WHERE run_id=$1 AND type='run.completed') FROM runs WHERE id=$1`, id).Scan(&status, &messages, &terminals); err != nil {
		t.Fatal(err)
	}
	if status != "running" || messages != 0 || terminals != 0 {
		t.Fatalf("终态事务部分提交: status=%s messages=%d terminals=%d", status, messages, terminals)
	}
	if _, err := db.Exec(t.Context(), `DROP TRIGGER reject_p6_terminal ON run_events; DROP FUNCTION reject_p6_terminal()`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE runs SET lease_expires_at=now()-interval '2 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	(&run.Sweeper{DB: db, Runs: svc, Stale: time.Second}).TickOnce(t.Context())
	var parts []byte
	if err := db.QueryRow(t.Context(), `SELECT r.status,m.parts FROM runs r JOIN messages m ON m.id=r.final_message_id WHERE r.id=$1`, id).Scan(&status, &parts); err != nil {
		t.Fatal(err)
	}
	if status != "interrupted" || len(parts) == 0 {
		t.Fatal("故障恢复后未完整收尾")
	}
}

// TestP6EventBatchRetry 验证批次中间写入失败时后续事件和原序号不会丢失。
func TestP6EventBatchRetry(t *testing.T) {
	db := newP5Database(t)
	seedP6Queries(t, db)
	store := run.NewPGStore(db)
	bus := run.NewEventBus("p6_r2", nil, nil, store)
	defer bus.Close()
	if _, err := db.Exec(t.Context(), `CREATE FUNCTION reject_p6_tool() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='tool.called' THEN RAISE EXCEPTION 'P6 tool event rejected'; END IF; RETURN NEW; END $$;
CREATE TRIGGER reject_p6_tool BEFORE INSERT ON run_events FOR EACH ROW EXECUTE FUNCTION reject_p6_tool()`); err != nil {
		t.Fatal(err)
	}
	if err := bus.MessageDelta(t.Context(), "正文"); err != nil {
		t.Fatal(err)
	}
	if err := bus.Emit(t.Context(), run.EventToolCalled, map[string]any{"call_id": "retry", "name": "current_time"}); err == nil {
		t.Fatal("数据库故障未返回")
	}
	if _, err := db.Exec(t.Context(), `DROP TRIGGER reject_p6_tool ON run_events; DROP FUNCTION reject_p6_tool()`); err != nil {
		t.Fatal(err)
	}
	if err := bus.Emit(t.Context(), run.EventToolCompleted, map[string]any{"call_id": "retry", "name": "current_time", "status": "succeeded"}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListRunEventsAfter(t.Context(), "p6_r2", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{run.EventMessageDelta, run.EventToolCalled, run.EventToolCompleted}
	if len(events) != len(want) {
		t.Fatal("批次重试丢失事件", len(events))
	}
	for i, event := range events {
		if event.Seq != int64(i+1) || event.Type != want[i] {
			t.Fatalf("批次重试顺序错误: %d %s", event.Seq, event.Type)
		}
	}
}
