package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/framework/redact"
	"linq-bs/server/internal/tool"
)

// Approval 保存单次调用的审批依据，参数只通过 args_hash 核对。
type Approval struct {
	ID               string             `json:"approval_id"`
	RunID            string             `json:"run_id"`
	CallID           string             `json:"call_id"`
	Name             string             `json:"name"`
	Title            string             `json:"title"`
	Summary          string             `json:"summary"`
	Items            []tool.DisplayItem `json:"items,omitempty"`
	ArgsPreview      string             `json:"args_preview,omitempty"`
	ArgsHash         string             `json:"-"`
	Status           string             `json:"status"`
	Reason           string             `json:"reason,omitempty"`
	Remember         string             `json:"remember,omitempty"`
	CanRememberAgent bool               `json:"can_remember_agent"`
	ExpiresAt        time.Time          `json:"expires_at"`
}

// ResumeState 保存无法从最终消息无损推断的引擎状态。
type ResumeState struct {
	VisibleText    string         `json:"visible_text"`
	EnableThinking bool           `json:"enable_thinking"`
	Step           int            `json:"step"`
	Turns          int            `json:"turns"`
	ToolCalls      int            `json:"tool_calls"`
	TotalTokens    int            `json:"total_tokens"`
	WrapUpEntered  bool           `json:"wrap_up_entered"`
	FailedSteps    int            `json:"failed_steps"`
	CallHashes     map[string]int `json:"call_hashes"`
	PerToolCalls   map[string]int `json:"per_tool_calls"`
}

func (rc *RunContext) checkpoint() RunContextSnapshot {
	return RunContextSnapshot{RunID: rc.Run.ID, Messages: rc.Messages, Segments: rc.Segments, Resume: ResumeState{
		VisibleText: rc.VisibleText.String(), EnableThinking: rc.EnableThinking, Step: rc.Step,
		Turns: rc.Budget.Turns, ToolCalls: rc.Budget.ToolCalls, TotalTokens: rc.Budget.TotalTokens,
		WrapUpEntered: rc.Budget.WrapUpEntered, FailedSteps: rc.Budget.ConsecutiveFailSteps,
		CallHashes: rc.CallHashes, PerToolCalls: rc.PerToolCalls,
	}}
}

// approvalGate 先核对调用级决定，再核对对话授予，最后才创建断点。
func approvalGate(ctx context.Context, rc *RunContext, pc PlannedCall) (bool, error) {
	entry, _ := rc.Toolset.EntryByName(pc.Call.Name)
	if entry.Approval != tool.ApprovalAlways {
		return false, nil
	}
	row, err := rc.Store.GetToolResultByCallID(ctx, rc.Run.ID, pc.Call.ID)
	if err != nil {
		return false, err
	}
	a, err := rc.Store.ApprovalForCall(ctx, rc.Run.ID, pc.Call.ID)
	if err != nil {
		return false, err
	}
	if a != nil {
		if rc.Tracer != nil && a.Status != "pending" {
			rc.Tracer.closeApproval(a.CallID, a.Status, a.Remember, time.Now().UTC())
		}
		if argsHash(row.ToolName, row.Arguments) != a.ArgsHash || argsHash(pc.Call.Name, []byte(pc.Call.Arguments)) != a.ArgsHash {
			return false, completeApprovalError(ctx, rc, row, "args_mismatch", "审批参数已发生变化，操作未执行。")
		}
		switch a.Status {
		case "approved":
			return false, nil
		case "rejected", "expired":
			code := "approval_rejected"
			message := "用户拒绝了该操作，不要再次尝试同一操作，除非用户再次明确要求。"
			if a.Status == "expired" {
				code = "approval_expired"
				message = "用户未在时限内确认，不要再次尝试同一操作，除非用户再次明确要求。"
			}
			if a.Reason != "" {
				message += " 用户说明为“" + redact.String(a.Reason) + "”。"
			}
			return false, completeApprovalError(ctx, rc, row, code, message)
		default:
			return false, fmt.Errorf("审批状态不允许执行 %s", a.Status)
		}
	}
	allowed, err := rc.Store.ConversationAllows(ctx, rc.Env.ConversationID, rc.Env.Principal.UserID, pc.Call.Name)
	if err != nil || allowed {
		return false, err
	}
	display := tool.Display{}
	impl, ok := rc.Registry.Get(pc.Call.Name)
	if !ok {
		return false, fmt.Errorf("工具未注册 %s", pc.Call.Name)
	}
	if preview, ok := impl.(tool.Previewable); ok {
		display, err = preview.Preview(ctx, tool.Call{ID: pc.Call.ID, Name: pc.Call.Name, Arguments: row.Arguments, Step: rc.Step}, rc.Env)
		if err != nil {
			var te *tool.Error
			if !errors.As(err, &te) {
				return false, err
			}
			return false, completeApprovalError(ctx, rc, row, te.Code, te.Message)
		}
	}
	for i := range display.Items {
		display.Items[i].Label = truncateRunes(redact.String(display.Items[i].Label), 80)
		display.Items[i].Value = truncateRunes(redact.String(display.Items[i].Value), 80)
	}
	ttl := rc.ApprovalTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	a = &Approval{ID: "appr_" + uuid.NewString(), RunID: rc.Run.ID, CallID: pc.Call.ID,
		Name: pc.Call.Name, Title: entry.Spec.Title, ArgsHash: row.ArgsHash, Status: "pending",
		Summary: truncateRunes(redact.String(display.Summary), 80), Items: display.Items,
		ArgsPreview: truncateRunes(redact.String(pc.Call.Arguments), 512), ExpiresAt: time.Now().Add(ttl),
		CanRememberAgent: entry.Spec.Approval.Overridable && rc.CanManageAgent,
	}
	if err := rc.Store.SuspendApproval(ctx, rc, row, *a); err != nil {
		return false, err
	}
	return true, nil
}

func completeApprovalError(ctx context.Context, rc *RunContext, row ToolResultRow, code, message string) error {
	rc.Budget.AddToolBudget()
	rc.PerToolCalls[row.ToolName]++
	row.Status = "rejected"
	row.ErrorCode, row.ErrorMessage = code, message
	row.ModelContent = FormatToolError(&tool.Error{Code: code, Message: message, Retryable: false})
	if err := rc.withToolTx(ctx, func(tx pgx.Tx) error { return rc.Store.UpdateToolResult(ctx, tx, row) }); err != nil {
		return err
	}
	entry, _ := rc.Toolset.EntryByName(row.ToolName)
	if err := rc.recordTool(row, time.Now()); err != nil {
		return err
	}
	emitToolCompleted(ctx, rc, row, entry, 0)
	return appendToolMessage(ctx, rc, PlannedCall{Call: toolCallFromRow(row)}, row.ModelContent)
}

// approvalBoundary 把挂起事件和断点交给同一个事务提交，避免重启后漏掉 required。
func approvalBoundary(ctx context.Context, rc *RunContext, a Approval, commit func([]StoredEvent) error) error {
	bus := rc.Bus
	if bus == nil {
		return commit(nil)
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.flushDeltasLocked()
	bus.flushBatchLocked(false)
	if bus.persistenceErr != nil {
		return bus.persistenceErr
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	payloads := []map[string]any{payload, {"reason": "approval", "approval_ids": []string{a.ID}}}
	types := []string{EventApprovalRequired, EventRunWaiting}
	events := make([]StoredEvent, 2)
	for i := range events {
		seq := bus.seq + int64(i) + 1
		raw, err := MarshalEnvelope(seq, rc.Run.ID, types[i], time.Now(), rc.Step, payloads[i])
		if err != nil {
			return err
		}
		events[i] = StoredEvent{Seq: seq, Type: types[i], Payload: raw}
	}
	if err := commit(events); err != nil {
		return err
	}
	for _, ev := range events {
		bus.seq = ev.Seq
		pe := pendingEvent{seq: ev.Seq, typ: ev.Type, payload: ev.Payload}
		bus.publishRedis(ctx, pe)
		bus.notifySubscribers(pe)
	}
	return nil
}
