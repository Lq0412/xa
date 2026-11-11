package run

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sweeper 周期性回收僵尸 run。
type Sweeper struct {
	lastRetention time.Time
	DB            *pgxpool.Pool
	Interval      time.Duration
	Stale         time.Duration
	Runs          *RunService
}

// Run 阻塞运行 sweeper，直到 ctx 取消。
func (sw *Sweeper) Run(ctx context.Context) {
	if sw.Interval <= 0 {
		sw.Interval = 15 * time.Second
	}
	if sw.Stale <= 0 {
		sw.Stale = 30 * time.Second
	}
	ticker := time.NewTicker(sw.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sw.tick(ctx)
		}
	}
}

// TickOnce 执行一轮回收，供集成测试调用。
func (sw *Sweeper) TickOnce(ctx context.Context) {
	sw.tick(ctx)
}

func (sw *Sweeper) tick(ctx context.Context) {
	if sw.Runs != nil {
		if err := sw.Runs.ExpireApprovals(ctx); err != nil {
			slog.Error("审批超时处理失败", "error", err)
		}
	}
	conn, err := sw.DB.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('xa_run_sweeper'))`).Scan(&locked); err != nil || !locked {
		return
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtext('xa_run_sweeper'))`); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
	}()
	rows, err := conn.Query(ctx, `
SELECT id, conversation_id FROM runs
WHERE status='running' AND lease_expires_at < now() - make_interval(secs => $1)`, sw.Stale.Seconds())
	if err != nil {
		return
	}
	type staleRun struct{ id, conv string }
	var stale []staleRun
	for rows.Next() {
		var id, conv string
		if err := rows.Scan(&id, &conv); err != nil {
			continue
		}
		stale = append(stale, staleRun{id, conv})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return
	}
	for _, r := range stale {
		if err := sw.interrupt(ctx, r.id, r.conv); err != nil {
			slog.Error("回收中断轮次失败", "run_id", r.id, "error", err)
		}
	}
	if sw.Runs != nil && time.Since(sw.lastRetention) >= 10*time.Minute {
		if err := sw.Runs.CleanupRetention(ctx); err != nil {
			slog.Error("运行保留期清理失败", "error", err)
		} else {
			sw.lastRetention = time.Now()
		}
	}
}

// interrupt 把中断消息、事件、追踪和快照作为一个收尾事务保存。
func (sw *Sweeper) interrupt(ctx context.Context, runID, convID string) error {
	tx, err := sw.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var seq int64
	// 候选扫描与回收间可能已续租，锁住 run 后再核对一次租约。
	if err := tx.QueryRow(ctx, `SELECT last_event_seq FROM runs WHERE id=$1 AND status='running' AND lease_expires_at<now()-make_interval(secs=>$2) FOR UPDATE`, runID, sw.Stale.Seconds()).Scan(&seq); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	// 旧进程可能在写入事件后、更新游标前退出。持锁后从实际事件末尾续写。
	if err := tx.QueryRow(ctx, `SELECT GREATEST($2::bigint, COALESCE(MAX(seq),0)) FROM run_events WHERE run_id=$1`, runID, seq).Scan(&seq); err != nil {
		return err
	}
	service := sw.Runs
	if service == nil {
		service = &RunService{DB: sw.DB, Store: NewPGStore(sw.DB)}
	}
	r, err := service.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	rc := &RunContext{Run: r, Store: service.Store, Segments: []byte("[]")}
	spans, err := service.Store.ListSpans(ctx, runID)
	if err != nil {
		return err
	}
	if len(spans) > 0 {
		rc.Tracer = NewTracer(r, PGSpanSink{Store: service.Store}, spans)
		// 模型节点逐步落盘，runs 的计数只在审批或收尾写入。崩溃时补齐已完成调用。
		var prompt, completion, cached, turns int
		for _, span := range spans {
			if span.Kind == "llm" && span.PromptTokens != nil && span.CompletionTokens != nil && span.CachedTokens != nil {
				prompt += *span.PromptTokens
				completion += *span.CompletionTokens
				cached += *span.CachedTokens
				turns++
				rc.Run.Steps = max(rc.Run.Steps, span.Step)
			}
		}
		rc.Run.PromptTokens = max(rc.Run.PromptTokens, prompt)
		rc.Run.CompletionTokens = max(rc.Run.CompletionTokens, completion)
		rc.Run.CachedTokens = max(rc.Run.CachedTokens, cached)
		rc.Run.ModelTurns = max(rc.Run.ModelTurns, turns)
	}
	rc.Step = rc.Run.Steps
	events, err := service.Store.ListRunEventsAfter(ctx, runID, 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Type == EventMessageDelta {
			var envelope struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal(event.Payload, &envelope); err != nil {
				return err
			}
			rc.VisibleText.WriteString(envelope.Delta)
		}
	}
	if rc.VisibleText.Len() == 0 {
		messages, err := service.Store.ListRunMessages(ctx, runID)
		if err != nil {
			return err
		}
		for _, msg := range messages {
			if msg.Role == "assistant" && msg.Content != "" {
				if rc.VisibleText.Len() > 0 {
					rc.VisibleText.WriteString("\n\n")
				}
				rc.VisibleText.WriteString(msg.Content)
			}
		}
	}
	var terminalEvents []pendingEvent
	writeEvent := func(typ string, payload map[string]any) error {
		seq++
		raw, err := MarshalEnvelope(seq, runID, typ, time.Now().UTC(), rc.Step, payload)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO run_events(run_id,seq,type,payload) VALUES($1,$2,$3,$4)`, runID, seq, typ, raw); err != nil {
			return err
		}
		terminalEvents = append(terminalEvents, pendingEvent{seq: seq, typ: typ, payload: raw, terminal: IsTerminalEvent(typ)})
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT call_id,tool_name,status,effect FROM tool_results WHERE run_id=$1 AND status IN ('pending','awaiting_approval','running') ORDER BY step,call_index`, runID)
	if err != nil {
		return err
	}
	type pending struct{ call, name, status, effect string }
	var pendingCalls []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.call, &p.name, &p.status, &p.effect); err != nil {
			rows.Close()
			return err
		}
		pendingCalls = append(pendingCalls, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range pendingCalls {
		if rc.Tracer != nil {
			rc.Tracer.closeApproval(p.call, "cancelled", "", time.Now().UTC())
			span := rc.Tracer.Start(rc.Tracer.RootID, "tool", "execute_tool "+p.name, rc.Step, p.call, time.Now().UTC())
			rc.Tracer.End(span, "error", "interrupted", time.Now().UTC())
		}
		resultStatus := "cancelled"
		if p.status == "running" && p.effect != "read" {
			resultStatus = "unknown"
		}
		content := FormatRejectResult("interrupted", "服务中断，操作未完成")
		if _, err := tx.Exec(ctx, `UPDATE tool_results SET status=$3,error_code='interrupted',model_content=$4,ended_at=now() WHERE run_id=$1 AND call_id=$2`, runID, p.call, resultStatus, content); err != nil {
			return err
		}
		ord, err := service.Store.NextOrdinal(ctx, tx, runID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO run_messages(id,run_id,step,ordinal,role,content,tool_call_id,tool_name) VALUES($1,$2,$3,$4,'tool',$5,$6,$7) ON CONFLICT(run_id,tool_call_id) WHERE role='tool' DO NOTHING`, "rm_"+uuid.NewString(), runID, rc.Step, ord, content, p.call, p.name); err != nil {
			return err
		}
		if err := writeEvent(EventToolCompleted, map[string]any{"call_id": p.call, "name": p.name, "status": resultStatus}); err != nil {
			return err
		}
	}
	approvalRows, err := tx.Query(ctx, `UPDATE run_approvals SET status='cancelled',decided_at=now() WHERE run_id=$1 AND status='pending' RETURNING id,call_id`, runID)
	if err != nil {
		return err
	}
	type closedApproval struct{ id, call string }
	var closed []closedApproval
	for approvalRows.Next() {
		var a closedApproval
		if err := approvalRows.Scan(&a.id, &a.call); err != nil {
			approvalRows.Close()
			return err
		}
		closed = append(closed, a)
	}
	approvalRows.Close()
	if err := approvalRows.Err(); err != nil {
		return err
	}
	for _, a := range closed {
		if err := writeEvent(EventApprovalResolved, map[string]any{"approval_id": a.id, "call_id": a.call, "decision": "cancelled"}); err != nil {
			return err
		}
	}
	messageID := "msg_" + uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,model,status,error,agent_version_id,run_id,context_segments) VALUES($1,$2,'assistant',$3,$4,'failed','服务中断，本次回复未完成',$5,$6,$7)`, messageID, convID, rc.VisibleText.String(), rc.Run.Model, rc.Run.AgentVersionID, runID, rc.Segments); err != nil {
		return err
	}
	if err := writeEvent(EventRunFailed, map[string]any{"message_id": messageID, "status": "interrupted", "code": "interrupted", "message": "服务中断，本次回复未完成"}); err != nil {
		return err
	}
	ended := time.Now().UTC()
	rc.Run.Status = "interrupted"
	rc.Run.ErrorCode = "interrupted"
	rc.Run.EndedAt = &ended
	if rc.Tracer != nil {
		rc.Tracer.Finish(rc.Run)
		if err := service.Store.UpsertSpans(ctx, tx, rc.Tracer.Spans()); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET status='interrupted',error='服务中断，本次回复未完成',error_code='interrupted',ended_at=$2,final_message_id=$3,last_event_seq=$4,prompt_tokens=$5,completion_tokens=$6,cached_tokens=$7,model_turns=$8,steps=$9,latency_ms=$10,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, runID, ended, messageID, seq, rc.Run.PromptTokens, rc.Run.CompletionTokens, rc.Run.CachedTokens, rc.Run.ModelTurns, rc.Run.Steps, max(0, ended.Sub(rc.Run.StartedAt).Milliseconds())); err != nil {
		return err
	}
	if service.SnapshotParts != nil {
		if err := service.SnapshotParts(ctx, tx, runID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	bus := service.newEventBus(runID)
	for _, ev := range terminalEvents {
		bus.publishRedis(ctx, ev)
	}
	return nil
}
