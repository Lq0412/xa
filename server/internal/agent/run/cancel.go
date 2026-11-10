package run

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
)

// closePendingCalls 为已经结束的轮次补齐尚未执行的工具消息。
func (s *RunService) closePendingCalls(ctx context.Context, rc *RunContext, tx pgx.Tx) ([]ToolResultRow, error) {
	pending, err := s.Store.PendingCallsOfLastStep(ctx, rc.Run.ID)
	if err != nil {
		return nil, err
	}
	rc.Tx = tx
	defer func() { rc.Tx = nil }()
	var rows []ToolResultRow
	for _, pc := range pending {
		row, err := rc.toolResult(ctx, pc.Call.ID)
		if err != nil {
			return nil, err
		}
		row.Status, row.ErrorCode, row.ErrorMessage = "cancelled", "cancelled", "操作未执行"
		row.ModelContent = FormatRejectResult(row.ErrorCode, row.ErrorMessage)
		if err := s.Store.UpdateToolResult(ctx, tx, row); err != nil {
			return nil, err
		}
		if err := appendToolMessage(ctx, rc, pc, row.ModelContent); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Cancel 对等待中的 run 同步收尾，运行中的 run 通过现有 watcher 停止。
func (s *RunService) Cancel(ctx context.Context, p auth.Principal, id string) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var owner, status, initialStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1 AND owner_user_id=$2`, id, p.UserID).Scan(&initialStatus); err != nil {
		if err == pgx.ErrNoRows {
			return "", definition.ErrNotFound
		}
		return "", err
	}
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT owner_user_id,status,last_event_seq FROM runs WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &status, &seq); err != nil {
		if err == pgx.ErrNoRows {
			return "", definition.ErrNotFound
		}
		return "", err
	}
	if owner != p.UserID {
		return "", definition.ErrNotFound
	}
	if initialStatus == "waiting_approval" && status == "running" {
		return "", approvalConflict("run_not_waiting", "审批已决定，请再次停止正在执行的操作")
	}
	if status != "running" && status != "waiting_approval" {
		return status, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET cancel_requested_at=now(),cancel_reason=$2 WHERE id=$1`, id, map[bool]string{true: "approval_cancel", false: "user_cancel"}[status == "waiting_approval"]); err != nil {
		return "", err
	}
	if status == "running" {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		if s.Coord == nil {
			s.Coord = NewCoordinator()
		}
		return "cancelling", s.Coord.CancelRun(ctx, s.Redis, id, "user_cancel")
	}
	rc, err := s.restoreContext(ctx, id, nil, nil)
	if err != nil {
		return "", err
	}
	rc.Tx = tx
	pending, err := s.Store.PendingCallsOfLastStep(ctx, id)
	if err != nil {
		return "", err
	}
	var events []pendingEvent
	writeEvent := func(typ string, payload map[string]any) error {
		seq++
		raw, err := MarshalEnvelope(seq, id, typ, time.Now(), rc.Step, payload)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO run_events(run_id,seq,type,payload) VALUES($1,$2,$3,$4)`, id, seq, typ, raw)
		events = append(events, pendingEvent{seq: seq, typ: typ, payload: raw, terminal: IsTerminalEvent(typ)})
		return err
	}
	for _, pc := range pending {
		row, err := rc.toolResult(ctx, pc.Call.ID)
		if err != nil {
			return "", err
		}
		var aid string
		err = tx.QueryRow(ctx, `UPDATE run_approvals SET status='cancelled',decided_at=now() WHERE run_id=$1 AND call_id=$2 AND status='pending' RETURNING id`, id, pc.Call.ID).Scan(&aid)
		if err != nil && err != pgx.ErrNoRows {
			return "", err
		}
		if aid != "" {
			if err := writeEvent(EventApprovalResolved, map[string]any{"approval_id": aid, "call_id": pc.Call.ID, "decision": "cancelled"}); err != nil {
				return "", err
			}
		}
		row.Status, row.ErrorCode, row.ErrorMessage = "cancelled", "cancelled", "已停止生成"
		row.ModelContent = FormatRejectResult("cancelled", "已停止生成")
		if err := s.Store.UpdateToolResult(ctx, tx, row); err != nil {
			return "", err
		}
		if err := appendToolMessage(ctx, rc, pc, row.ModelContent); err != nil {
			return "", err
		}
		if err := writeEvent(EventToolCompleted, map[string]any{"call_id": pc.Call.ID, "name": pc.Call.Name, "status": "cancelled"}); err != nil {
			return "", err
		}
	}
	messageID := "msg_" + uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,model,status,error,agent_version_id,run_id,context_segments) VALUES($1,$2,'assistant',$3,$4,'failed','已停止生成',$5,$6,$7)`, messageID, rc.Run.ConversationID, rc.VisibleText.String(), rc.Run.Model, rc.Run.AgentVersionID, id, rc.Segments); err != nil {
		return "", err
	}
	if err := s.Store.SaveRunContext(ctx, tx, rc.checkpoint()); err != nil {
		return "", err
	}
	if err := writeEvent(EventRunCancelled, map[string]any{"message_id": messageID, "status": "cancelled", "code": "approval_cancel", "message": "已停止生成"}); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET status='cancelled',final_message_id=$2,ended_at=now(),last_event_seq=$3,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND status='waiting_approval'`, id, messageID, seq); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	bus := NewEventBus(id, s.Redis, s.RedisOK, s.Store)
	for _, ev := range events {
		bus.publishRedis(ctx, ev)
	}
	return "cancelled", nil
}
