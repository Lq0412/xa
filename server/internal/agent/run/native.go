package run

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

// NativeEngine 实现显式工具循环。
type NativeEngine struct {
	Scheduler Scheduler
}

var _ Engine = (*NativeEngine)(nil)

// Execute 从 rc 当前状态推进到终态。
func (e *NativeEngine) Execute(ctx context.Context, rc *RunContext) (Outcome, error) {
	if rc.Budget == nil {
		rc.Budget = NewBudget(rc.Limits)
	}
	if rc.WallStarted.IsZero() {
		rc.WallStarted = time.Now()
	}
	sch := e.Scheduler
	pending, err := rc.Store.PendingCallsOfLastStep(ctx, rc.Run.ID)
	if err != nil {
		return Outcome{}, err
	}
	if len(pending) > 0 {
		if err := rc.Budget.HardCheck(ActiveMS(rc.WallStarted)); err != nil {
			return Outcome{Kind: OutcomeBudgetExhausted, Code: "wall_time"}, nil
		}
		out, err := sch.runPending(ctx, rc, pending)
		if err != nil {
			return Outcome{}, err
		}
		if out.Suspended {
			return Outcome{Kind: OutcomeWaitingApproval}, nil
		}
		rc.Budget.RecordToolStepFailure(out.AllFailed)
	}
	for {
		activeMS := ActiveMS(rc.WallStarted)
		if err := rc.Budget.HardCheck(activeMS); err != nil {
			return Outcome{Kind: OutcomeBudgetExhausted, Code: "wall_time", Message: "已达到本轮资源上限"}, nil
		}
		wrapUp := rc.Budget.NeedWrapUp()
		if wrapUp {
			rc.Budget.MarkWrapUp()
		}
		toolSchemaTok := 0
		tools := rc.Toolset.ModelTools()
		if len(tools) > 0 && !wrapUp {
			raw, _ := json.Marshal(tools)
			toolSchemaTok = estimateMessagesTokens([]provider.Message{{Role: "system", Content: string(raw)}})
		}
		maxOut := rc.Limits.MaxOutputTokens
		if hard, stop, adj := rc.Budget.TokenPreCheck(rc.Messages, toolSchemaTok, maxOut); stop {
			return Outcome{Kind: OutcomeBudgetExhausted, Code: "tokens", Message: "已达到本轮资源上限"}, nil
		} else if hard {
			wrapUp = true
			rc.Budget.MarkWrapUp()
			maxOut = adj
		}
		req := provider.ChatRequest{
			Model: rc.Run.Model, Messages: rc.ModelMessages(wrapUp), MaxOutputTokens: maxOut,
			ToolChoice: provider.ToolChoice{Mode: provider.ToolChoiceAuto},
		}
		if !wrapUp && len(tools) > 0 {
			req.Tools = tools
			par := rc.Limits.MaxParallelTools > 1
			req.ParallelToolCalls = &par
		}
		rc.Step++
		if rc.Emitter != nil {
			_ = rc.Emitter.Emit(ctx, EventStepStarted, map[string]any{"step": rc.Step})
		}
		if rc.VisibleText.Len() > 0 && rc.Emitter != nil {
			rc.Emitter.PendingSeparator("\n\n")
		}
		req.EnableThinking = rc.EnableThinking
		resp, err := callModelWithRetry(ctx, rc, req, streamSink(ctx, rc))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return Outcome{Kind: OutcomeCancelled, Code: "cancelled", Message: "已停止生成"}, nil
			}
			var pe *provider.ProviderError
			if errors.As(err, &pe) && pe.Kind == provider.ProviderErrorContentFilter {
				return Outcome{Kind: OutcomeFailed, Code: "content_filtered", Message: "内容未通过安全审核"}, nil
			}
			return Outcome{Kind: OutcomeFailed, Code: "model_error", Message: "模型调用失败"}, err
		}
		rc.Budget.AddModelTurn(resp.Usage)
		rc.Run.PromptTokens += resp.Usage.PromptTokens
		rc.Run.CompletionTokens += resp.Usage.CompletionTokens
		rc.Run.CachedTokens += resp.Usage.CachedTokens
		calls := resp.Message.ToolCalls
		if wrapUp && len(calls) > 0 {
			calls = nil
			resp.Message.ToolCalls = nil
		}
		if err := appendAssistantStep(ctx, rc, resp); err != nil {
			return Outcome{}, err
		}
		if rc.Bus != nil {
			_ = rc.Bus.ReasoningCompleted(ctx, resp.FinishReason == provider.FinishLength)
		}
		emitStepCompleted(ctx, rc, resp)
		if len(calls) == 0 {
			if resp.FinishReason == provider.FinishContentFilter {
				return Outcome{Kind: OutcomeFailed, Code: "content_filtered", Message: "内容未通过安全审核"}, nil
			}
			return Outcome{Kind: OutcomeCompleted, Truncated: resp.FinishReason == provider.FinishLength}, nil
		}
		plan := Validate(calls, rc)
		for _, pc := range plan.Items {
			emitToolCalled(ctx, rc, pc)
		}
		if err := insertRejectedResults(ctx, rc, plan); err != nil {
			return Outcome{}, err
		}
		out, err := sch.Run(ctx, rc, plan.AcceptedCalls())
		if err != nil {
			if out.Cancelled || errors.Is(err, context.Canceled) {
				return Outcome{Kind: OutcomeCancelled, Code: "cancelled", Message: "已停止生成"}, nil
			}
			return Outcome{}, err
		}
		if out.Suspended {
			return Outcome{Kind: OutcomeWaitingApproval}, nil
		}
		rc.Budget.RecordToolStepFailure(out.AllFailed)
	}
}

func appendAssistantStep(ctx context.Context, rc *RunContext, resp provider.Response) error {
	rc.appendMessage(provider.Message{
		Role: resp.Message.Role, Content: resp.Message.Content, ToolCalls: resp.Message.ToolCalls,
	})
	return rc.appendRunMessage(ctx, RunMessage{
		RunID: rc.Run.ID, Step: rc.Step, Role: "assistant",
		Content: resp.Message.Content, ToolCalls: resp.Message.ToolCalls,
		FinishReason: string(resp.FinishReason),
	})
}

func insertRejectedResults(ctx context.Context, rc *RunContext, plan CallPlan) error {
	for _, pc := range plan.Items {
		if pc.Accepted {
			continue
		}
		content := FormatRejectResult(pc.ErrorCode, pc.ErrorMessage)
		if pc.CountsBudget {
			rc.Budget.AddToolBudget()
		}
		rc.appendMessage(toolMessage(pc.Call.Name, pc.Call.ID, content))
		if err := rc.appendRunMessage(ctx, RunMessage{
			RunID: rc.Run.ID, Step: rc.Step, Role: "tool",
			Content: content, ToolCallID: pc.Call.ID, ToolName: pc.Call.Name,
		}); err != nil {
			return err
		}
		ref := NewResultRef()
		args := []byte(pc.Call.Arguments)
		if len(args) == 0 {
			args = []byte("{}")
		}
		effect := string(tool.EffectRead)
		toolVersion := ""
		if entry, ok := rc.Toolset.EntryByName(pc.Call.Name); ok {
			effect = string(entry.Spec.Effect)
			toolVersion = entry.Spec.Version
		}
		row := ToolResultRow{
			ID: ref, RunID: rc.Run.ID, ConversationID: rc.Env.ConversationID,
			OwnerUserID: rc.Env.Principal.UserID, Step: rc.Step, CallIndex: pc.Index + 1, CallID: pc.Call.ID,
			ToolName: pc.Call.Name, ToolVersion: toolVersion, Effect: effect,
			Arguments: args, RawArguments: string(args), ArgsHash: argsHash(pc.Call.Name, args),
			Status: "rejected", ErrorCode: pc.ErrorCode,
			ErrorMessage: pc.ErrorMessage, ModelContent: content,
		}
		if err := rc.withToolTx(ctx, func(tx pgx.Tx) error {
			return rc.Store.InsertToolResult(ctx, tx, row)
		}); err != nil {
			return err
		}
		entry, _ := rc.Toolset.EntryByName(pc.Call.Name)
		emitToolCompleted(ctx, rc, row, entry, 0)
	}
	return nil
}
