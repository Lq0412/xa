package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/redact"
	"linq-bs/server/internal/tool"
)

// DecisionInput 的记住范围仅用于允许，拒绝始终是本次调用。
type DecisionInput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Remember string `json:"remember"`
}

// DecisionResult 返回已经提交的决定与可选配置保存警告。
type DecisionResult struct {
	ApprovalID string `json:"approval_id"`
	Status     string `json:"status"`
	RunID      string `json:"run_id"`
	RunStatus  string `json:"run_status"`
	Warning    string `json:"warning,omitempty"`
}

// ApprovalError 携带决策 API 的稳定错误码。
type ApprovalError struct {
	Code    string
	Message string
	Status  int
}

func (e *ApprovalError) Error() string { return e.Message }

func approvalConflict(code, message string) error { return &ApprovalError{code, message, 409} }

func (s *RunService) restoreContext(ctx context.Context, id string, sink EventSink, bus *EventBus) (*RunContext, error) {
	r, err := s.Store.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	snap, err := s.Store.LoadRunContext(ctx, id)
	if err != nil {
		return nil, err
	}
	state := snap.Resume
	budget := NewBudget(r.Limits)
	budget.Turns, budget.ToolCalls, budget.TotalTokens = state.Turns, state.ToolCalls, state.TotalTokens
	budget.WrapUpEntered, budget.ConsecutiveFailSteps = state.WrapUpEntered, state.FailedSteps
	rc := &RunContext{Run: r, Toolset: r.Toolset, Messages: snap.Messages, Step: state.Step, Budget: budget,
		Store: s.Store, Registry: s.Registry, Provider: s.Provider, Emitter: sink, Bus: bus, Limits: r.Limits,
		EnableThinking: state.EnableThinking, WallStarted: time.Now().Add(-time.Duration(r.ActiveMS) * time.Millisecond),
		CallHashes: state.CallHashes, PerToolCalls: state.PerToolCalls, Segments: snap.Segments,
		ApprovalTTL: time.Duration(s.Config.ApprovalTTLSec) * time.Second, LeaseOwner: s.InstanceID,
		Env: tool.Env{Principal: auth.Principal{UserID: r.OwnerUserID, WorkspaceID: r.WorkspaceID}, WorkspaceID: r.WorkspaceID,
			AgentID: r.AgentID, AgentVersionID: r.AgentVersionID, ConversationID: r.ConversationID, RunID: r.ID, UserMessageID: r.UserMessageID},
	}
	if rc.CallHashes == nil {
		rc.CallHashes = map[string]int{}
	}
	if rc.PerToolCalls == nil {
		rc.PerToolCalls = map[string]int{}
	}
	if s.DB != nil {
		var role string
		if err := s.DB.QueryRow(ctx, `SELECT a.is_primary,u.role,COALESCE(a.owner_user_id=u.id,false) FROM agent_definitions a JOIN users u ON u.id=$2 WHERE a.id=$1`, r.AgentID, r.OwnerUserID).Scan(&rc.Env.IsPrimaryAgent, &role, &rc.CanManageAgent); err != nil {
			return nil, err
		}
		rc.Env.Principal.Roles = []string{role}
	}
	rc.VisibleText.WriteString(state.VisibleText)
	return rc, nil
}

// Decide 串行锁定 run 与审批行，在提交决定前取得会话写锁。
func (s *RunService) Decide(ctx context.Context, p auth.Principal, id string, in DecisionInput) (DecisionResult, error) {
	if in.Remember == "" {
		in.Remember = "once"
	}
	if (in.Decision != "approve" && in.Decision != "reject") || (in.Remember != "once" && in.Remember != "conversation" && in.Remember != "agent") || len([]rune(in.Reason)) > 200 || (in.Decision == "reject" && in.Remember != "once") {
		return DecisionResult{}, &ApprovalError{"invalid_decision", "审批决定无效", 400}
	}
	in.Reason = redact.String(in.Reason)
	result, err := s.decide(ctx, p, id, in, false)
	var conflict *ApprovalError
	if errors.As(err, &conflict) && conflict.Status == 409 {
		slog.Info("工具审批冲突", "approval_id", id, "code", conflict.Code)
	}
	return result, err
}

func (s *RunService) decide(ctx context.Context, p auth.Principal, id string, in DecisionInput, system bool) (DecisionResult, error) {
	var rid, owner string
	if err := s.DB.QueryRow(ctx, `SELECT run_id,owner_user_id FROM run_approvals WHERE id=$1`, id).Scan(&rid, &owner); err != nil {
		if err == pgx.ErrNoRows {
			return DecisionResult{}, definition.ErrNotFound
		}
		return DecisionResult{}, err
	}
	if !system && owner != p.UserID {
		return DecisionResult{}, definition.ErrNotFound
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return DecisionResult{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var runStatus, convID string
	var seq int64
	var cancelAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT status,conversation_id,last_event_seq,cancel_requested_at FROM runs WHERE id=$1 FOR UPDATE`, rid).Scan(&runStatus, &convID, &seq, &cancelAt); err != nil {
		return DecisionResult{}, err
	}
	var status, callID, name, remember, reason string
	var expires, created time.Time
	if err := tx.QueryRow(ctx, `SELECT status,call_id,tool_name,remember,reason,expires_at,created_at FROM run_approvals WHERE id=$1 FOR UPDATE`, id).Scan(&status, &callID, &name, &remember, &reason, &expires, &created); err != nil {
		return DecisionResult{}, err
	}
	want := "approved"
	if in.Decision == "reject" {
		want = "rejected"
	}
	result := DecisionResult{ApprovalID: id, Status: status, RunID: rid, RunStatus: runStatus}
	if status != "pending" {
		if !system && status == want && remember == in.Remember && reason == in.Reason {
			return result, nil
		}
		return result, approvalConflict("approval_conflict", "该操作已有其他决定")
	}
	if runStatus != "waiting_approval" || cancelAt != nil {
		return result, approvalConflict("run_not_waiting", "本轮已停止或正在执行")
	}
	expired := !expires.After(time.Now())
	if expired {
		want = "expired"
		in.Remember = ""
		in.Reason = ""
	}
	if system && !expired {
		return result, approvalConflict("approval_conflict", "审批尚未过期")
	}
	if s.Coord != nil && s.Coord.Bus(rid) != nil {
		return result, approvalConflict("conflict", "正在保存审批，请重试")
	}
	rc, err := s.restoreContext(ctx, rid, nil, nil)
	if err != nil {
		return result, err
	}
	token, err := acquireConvLock(ctx, s.Redis, convID, rid, s.leaseTTL())
	if err != nil {
		return result, approvalConflict("conflict", "会话写锁暂不可用，请重试")
	}
	committed := false
	defer func() {
		if !committed {
			releaseConvLock(context.WithoutCancel(ctx), s.Redis, convID, token)
		}
	}()
	var decider any = owner
	if expired {
		decider = nil
	}
	if _, err := tx.Exec(ctx, `UPDATE run_approvals SET status=$2,decided_by=$3,reason=$4,remember=$5,decided_at=now() WHERE id=$1 AND status='pending'`, id, want, decider, in.Reason, in.Remember); err != nil {
		return result, err
	}
	if want == "approved" && in.Remember == "conversation" {
		if _, err := tx.Exec(ctx, `INSERT INTO conversation_tool_grants(conversation_id,tool_name,owner_user_id,decision,source_approval_id) VALUES($1,$2,$3,'allow',$4) ON CONFLICT(conversation_id,tool_name) DO UPDATE SET source_approval_id=EXCLUDED.source_approval_id,updated_at=now()`, convID, name, owner, id); err != nil {
			return result, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET status='running',lease_owner=$2,lease_expires_at=now()+make_interval(secs=>$3),heartbeat_at=now(),updated_at=now() WHERE id=$1 AND status='waiting_approval'`, rid, s.InstanceID, s.leaseTTL().Seconds()); err != nil {
		return result, err
	}
	payload := map[string]any{"approval_id": id, "call_id": callID, "decision": want}
	if in.Remember != "" {
		payload["remember"] = in.Remember
	}
	raw, err := MarshalEnvelope(seq+1, rid, EventApprovalResolved, time.Now(), rc.Step, payload)
	if err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO run_events(run_id,seq,type,payload) VALUES($1,$2,$3,$4)`, rid, seq+1, EventApprovalResolved, raw); err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET last_event_seq=$2 WHERE id=$1`, rid, seq+1); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	committed = true
	bus := NewEventBus(rid, s.Redis, s.RedisOK, s.Store)
	bus.seq = seq + 1
	bus.publishRedis(ctx, pendingEvent{seq: bus.seq, typ: EventApprovalResolved, payload: raw})
	s.StartDrive(context.WithoutCancel(ctx), StreamStartInput{RunID: rid, StreamProtocol: rc.Run.StreamProtocol, resume: rc, writerToken: token, ChatStartInput: ChatStartInput{
		Conversation: ConversationRef{ID: convID}, Version: definition.AgentVersion{ID: rc.Run.AgentVersionID, Configuration: definition.Configuration{Model: rc.Run.Model}}, Segments: rc.Segments,
	}}, bus, nil)
	result.Status, result.RunStatus = want, "running"
	if want == "approved" && in.Remember == "agent" {
		settingsCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.rememberAgent(settingsCtx, p, rc.Run.AgentID, name); err != nil {
			result.Warning = "本次已允许，Agent 工具设置未保存。请在右侧面板重试。"
			slog.Warn("保存审批工具设置失败", "approval_id", id, "error", redact.String(err.Error()))
		}
	}
	slog.Info("工具审批已决定", "approval_id", id, "decision", want, "wait_ms", time.Since(created).Milliseconds())
	if expired && !system {
		return result, approvalConflict("approval_expired", "确认已过期，本轮将继续说明原因")
	}
	return result, nil
}

func (s *RunService) rememberAgent(ctx context.Context, p auth.Principal, agentID, name string) error {
	impl, ok := s.Registry.Get(name)
	if !ok || !impl.Spec().Approval.Overridable {
		return fmt.Errorf("该工具必须逐次确认")
	}
	if s.Agents == nil {
		return fmt.Errorf("Agent 服务未配置")
	}
	a, err := s.Agents.Get(ctx, p, agentID)
	if err != nil {
		return err
	}
	if a.OwnerUserID != p.UserID {
		return definition.ErrForbidden
	}
	v, err := s.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return err
	}
	found := false
	for i := range v.Configuration.Tools {
		if v.Configuration.Tools[i].Name == name {
			v.Configuration.Tools[i].Approval = "never"
			found = true
		}
	}
	if !found {
		v.Configuration.Tools = append(v.Configuration.Tools, binding.ToolBinding{Name: name, Enabled: true, Approval: "never"})
	}
	_, err = s.Agents.Save(ctx, p, definition.SaveInput{AgentID: a.ID, ExpectedVersionID: v.ID, Name: a.Name, Avatar: a.Avatar, Description: a.Description, Tags: a.Tags, Visibility: a.Visibility, Configuration: v.Configuration})
	return err
}

// ExpireApprovals 使用与用户决定相同的事务和恢复入口，支持服务重启后执行。
func (s *RunService) ExpireApprovals(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, `SELECT id FROM run_approvals WHERE status='pending' AND expires_at<=now() ORDER BY expires_at LIMIT 100`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.decide(ctx, auth.Principal{}, id, DecisionInput{}, true); err != nil {
			var conflict *ApprovalError
			if !errors.As(err, &conflict) {
				return err
			}
		}
	}
	return nil
}
