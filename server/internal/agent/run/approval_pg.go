package run

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// ApprovalForCall 返回已持久化的决定，缺失表示该调用尚未请求审批。
func (s *PGStore) ApprovalForCall(ctx context.Context, runID, callID string) (*Approval, error) {
	var a Approval
	var display []byte
	err := s.DB.QueryRow(ctx, `SELECT id,run_id,call_id,tool_name,args_hash,display,status,reason,remember,expires_at FROM run_approvals WHERE run_id=$1 AND call_id=$2`, runID, callID).
		Scan(&a.ID, &a.RunID, &a.CallID, &a.Name, &a.ArgsHash, &display, &a.Status, &a.Reason, &a.Remember, &a.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved Approval
	if err := json.Unmarshal(display, &saved); err != nil {
		return nil, err
	}
	a.Title, a.Summary, a.Items, a.ArgsPreview, a.CanRememberAgent = saved.Title, saved.Summary, saved.Items, saved.ArgsPreview, saved.CanRememberAgent
	return &a, nil
}

// ConversationAllows 仅匹配同一 owner 和会话，不改变工具集权限。
func (s *PGStore) ConversationAllows(ctx context.Context, conversationID, ownerID, toolName string) (bool, error) {
	var allowed bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_tool_grants WHERE conversation_id=$1 AND owner_user_id=$2 AND tool_name=$3 AND decision='allow')`, conversationID, ownerID, toolName).Scan(&allowed)
	return allowed, err
}

// SuspendApproval 同事务写入审批、引擎断点、run 状态及两条边界事件。
func (s *PGStore) SuspendApproval(ctx context.Context, rc *RunContext, row ToolResultRow, a Approval) error {
	return approvalBoundary(ctx, rc, a, func(events []StoredEvent) error {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		var status string
		var owner *string
		var cancelling bool
		if err := tx.QueryRow(ctx, `SELECT status,lease_owner,cancel_requested_at IS NOT NULL FROM runs WHERE id=$1 FOR UPDATE`, rc.Run.ID).Scan(&status, &owner, &cancelling); err != nil {
			return err
		}
		if cancelling {
			return context.Canceled
		}
		if status != "running" || (rc.LeaseOwner != "" && (owner == nil || *owner != rc.LeaseOwner)) {
			return fmt.Errorf("run 租约已失效")
		}
		display, err := json.Marshal(a)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO run_approvals(id,run_id,owner_user_id,call_id,tool_name,args_hash,display,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8)`, a.ID, rc.Run.ID, rc.Env.Principal.UserID, a.CallID, a.Name, a.ArgsHash, display, a.ExpiresAt)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE tool_results SET status='awaiting_approval' WHERE id=$1 AND status='pending'`, row.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("工具状态已变化")
		}
		if err := s.SaveRunContext(ctx, tx, rc.checkpoint()); err != nil {
			return err
		}
		if rc.Tracer != nil {
			span := rc.Tracer.Start(rc.Tracer.RootID, "approval", "approval "+a.Name, rc.Step, a.CallID, time.Now().UTC())
			span.Attributes = map[string]any{"approval_id": a.ID, "tool_name": a.Name, "decision": "pending"}
			rc.Tracer.Put(span)
			if err := s.UpsertSpans(ctx, tx, rc.Tracer.Spans()); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE runs SET status='waiting_approval',steps=$2,model_turns=$3,tool_calls=$4,prompt_tokens=$5,completion_tokens=$6,active_ms=$7,cached_tokens=$8,ttft_ms=$9,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND status='running'`, rc.Run.ID, rc.Step, rc.Budget.Turns, rc.Budget.ToolCalls, rc.Run.PromptTokens, rc.Run.CompletionTokens, ActiveMS(rc.WallStarted), rc.Run.CachedTokens, rc.Run.TTFTMS)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if _, err := tx.Exec(ctx, `INSERT INTO run_events(run_id,seq,type,payload) VALUES($1,$2,$3,$4)`, rc.Run.ID, ev.Seq, ev.Type, ev.Payload); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET last_event_seq=$2 WHERE id=$1`, rc.Run.ID, ev.Seq); err != nil {
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("工具等待审批", "run_id", a.RunID, "approval_id", a.ID, "tool", a.Name)
		return nil
	})
}
