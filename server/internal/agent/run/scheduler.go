package run

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/tool"
)

// SchedulerOutput 描述一步调度结果。
type SchedulerOutput struct {
	Cancelled bool
	AllFailed bool
}

// Scheduler 按批次执行工具调用；P3 不处理审批挂起。
type Scheduler struct{}

// Run 执行本步被接受的调用。
func (Scheduler) Run(ctx context.Context, rc *RunContext, accepted []PlannedCall) (SchedulerOutput, error) {
	if len(accepted) == 0 {
		return SchedulerOutput{}, nil
	}
	batches := planBatches(accepted, rc)
	out := SchedulerOutput{}
	for _, batch := range batches {
		if ctx.Err() != nil {
			out.Cancelled = true
			return out, ctx.Err()
		}
		if batch.parallel {
			failed, err := runParallelBatch(ctx, rc, batch.calls)
			if err != nil {
				if ctx.Err() != nil {
					out.Cancelled = true
				}
				return out, err
			}
			if failed == len(batch.calls) {
				out.AllFailed = true
			}
		} else {
			failed, err := runSerialBatch(ctx, rc, batch.calls[0])
			if err != nil {
				if ctx.Err() != nil {
					out.Cancelled = true
				}
				return out, err
			}
			if failed {
				out.AllFailed = true
			}
		}
	}
	return out, nil
}

type batchPlan struct {
	parallel bool
	calls    []PlannedCall
}

func planBatches(calls []PlannedCall, rc *RunContext) []batchPlan {
	var batches []batchPlan
	cur := []PlannedCall{}
	flushParallel := func() {
		if len(cur) > 0 {
			batches = append(batches, batchPlan{parallel: true, calls: append([]PlannedCall(nil), cur...)})
			cur = nil
		}
	}
	for _, c := range calls {
		entry, _ := rc.Toolset.EntryByName(c.Call.Name)
		if entry.Spec.Effect == tool.EffectRead && entry.Approval == tool.ApprovalNever {
			cur = append(cur, c)
		} else {
			flushParallel()
			batches = append(batches, batchPlan{parallel: false, calls: []PlannedCall{c}})
		}
	}
	flushParallel()
	return batches
}

func runParallelBatch(ctx context.Context, rc *RunContext, calls []PlannedCall) (failed int, err error) {
	cap := rc.Limits.MaxParallelTools
	if cap <= 0 {
		cap = 4
	}
	sem := make(chan struct{}, cap)
	type item struct {
		idx int
		pc  PlannedCall
		err error
	}
	results := make([]item, len(calls))
	var wg sync.WaitGroup
	for i, pc := range calls {
		wg.Add(1)
		go func(idx int, call PlannedCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			e := executeOne(ctx, rc, call)
			results[idx] = item{idx: idx, pc: call, err: e}
		}(i, pc)
	}
	wg.Wait()
	for i := 0; i < len(results); i++ {
		it := results[i]
		if it.err != nil {
			failed++
		}
		if it.err != nil && !errors.Is(it.err, context.Canceled) {
			err = it.err
		}
	}
	return failed, err
}

func runSerialBatch(ctx context.Context, rc *RunContext, call PlannedCall) (failed bool, err error) {
	if err := executeOne(ctx, rc, call); err != nil {
		if ctx.Err() != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func executeOne(ctx context.Context, rc *RunContext, pc PlannedCall) error {
	entry, ok := rc.Toolset.EntryByName(pc.Call.Name)
	if !ok {
		return errors.New("工具条目丢失")
	}
	toolImpl, ok := rc.Registry.Get(pc.Call.Name)
	if !ok {
		return errors.New("工具未注册")
	}
	rc.Budget.AddToolBudget()
	rc.PerToolCalls[pc.Call.Name]++
	args := []byte(pc.Call.Arguments)
	if len(args) == 0 {
		args = []byte("{}")
	}
	call := tool.Call{
		ID: pc.Call.ID, Name: pc.Call.Name, Arguments: args, Step: rc.Step,
	}
	env := rc.Env
	env.Config = entry.Config
	env.Deadline = time.Now().Add(entry.Timeout)
	resultRef := NewResultRef()
	row := ToolResultRow{
		ID: resultRef, RunID: rc.Run.ID, ConversationID: env.ConversationID, OwnerUserID: env.Principal.UserID,
		Step: rc.Step, CallID: pc.Call.ID, ToolName: pc.Call.Name, ToolVersion: entry.Spec.Version,
		Effect: string(entry.Spec.Effect), Arguments: args, RawArguments: string(args),
		ArgsHash: argsHash(pc.Call.Name, args), Status: "running", Attempts: 1,
	}
	tx, err := rc.Store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := rc.Store.InsertToolResult(ctx, tx, row); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	toolCtx, cancel := context.WithTimeout(ctx, minDuration(entry.Timeout, rc.RemainingWall()))
	defer cancel()

	var res tool.Result
	var execErr error
	if txTool, ok := toolImpl.(tool.TxTool); ok {
		prepared, prepErr := txTool.Prepare(toolCtx, call, env)
		if prepErr != nil {
			execErr = prepErr
		} else {
			tx2, err := rc.Store.BeginTx(ctx)
			if err != nil {
				return err
			}
			env.Tx = tx2
			res, execErr = txTool.ExecuteTx(toolCtx, tx2, call, env, prepared)
			if execErr == nil {
				execErr = persistToolSuccess(ctx, rc, tx2, &row, res, entry, resultRef)
			}
			if execErr != nil {
				_ = tx2.Rollback(ctx)
			} else {
				_ = tx2.Commit(ctx)
			}
			env.Tx = nil
		}
	} else {
		res, execErr = toolImpl.Execute(toolCtx, call, env)
		if execErr != nil && shouldRetryRead(toolImpl, execErr) {
			time.Sleep(300 * time.Millisecond)
			res, execErr = toolImpl.Execute(toolCtx, call, env)
		}
		if execErr == nil {
			tx2, err := rc.Store.BeginTx(ctx)
			if err != nil {
				return err
			}
			execErr = persistToolSuccess(ctx, rc, tx2, &row, res, entry, resultRef)
			if execErr != nil {
				_ = tx2.Rollback(ctx)
			} else {
				_ = tx2.Commit(ctx)
			}
		}
	}
	if execErr != nil {
		return persistToolFailure(ctx, rc, &row, execErr)
	}
	rc.Messages = append(rc.Messages, toolMessage(row.ToolName, row.CallID, row.ModelContent))
	return appendToolMessage(ctx, rc, pc, row.ModelContent)
}

func shouldRetryRead(t tool.Tool, err error) bool {
	if err == nil {
		return false
	}
	var te *tool.Error
	if !errors.As(err, &te) {
		return false
	}
	if te.Code != "tool_unavailable" && te.Code != "tool_timeout" {
		return false
	}
	return t.Spec().Idempotent && t.Spec().Effect == tool.EffectRead
}

func persistToolSuccess(ctx context.Context, rc *RunContext, tx pgx.Tx, row *ToolResultRow, res tool.Result, entry tool.Entry, ref string) error {
	modelContent, dataJSON, truncated, totalChars, resultRef := FormatSuccess(res, entry, rc.Limits.ToolResultMaxTok, func() string { return ref })
	dataJSON, _ = TruncateStoredData(dataJSON)
	row.ModelContent = modelContent
	row.Data = dataJSON
	row.Truncated = truncated
	row.TotalChars = totalChars
	row.Status = "succeeded"
	if resultRef != "" {
		row.ID = resultRef
	}
	disp, _ := json.Marshal(res.Display)
	row.Display = disp
	return rc.Store.UpdateToolResult(ctx, tx, *row)
}

func persistToolFailure(ctx context.Context, rc *RunContext, row *ToolResultRow, execErr error) error {
	var te *tool.Error
	content := FormatInternalError(execErr.Error())
	code := "internal_error"
	if errors.As(execErr, &te) {
		content = FormatToolError(te)
		code = te.Code
		row.ErrorMessage = te.Message
	}
	row.Status = "failed"
	row.ErrorCode = code
	row.ModelContent = content
	tx, err := rc.Store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := rc.Store.UpdateToolResult(ctx, tx, *row); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	rc.Messages = append(rc.Messages, toolMessage(row.ToolName, row.CallID, content))
	return rc.Store.AppendRunMessage(ctx, nil, RunMessage{
		RunID: rc.Run.ID, Step: rc.Step, Role: "tool", Content: content,
		ToolCallID: row.CallID, ToolName: row.ToolName,
	})
}

func appendToolMessage(ctx context.Context, rc *RunContext, pc PlannedCall, content string) error {
	rc.Messages = append(rc.Messages, toolMessage(pc.Call.Name, pc.Call.ID, content))
	ord, err := rc.Store.NextOrdinal(ctx, rc.Run.ID)
	if err != nil {
		return err
	}
	return rc.Store.AppendRunMessage(ctx, nil, RunMessage{
		RunID: rc.Run.ID, Step: rc.Step, Ordinal: ord, Role: "tool", Content: content,
		ToolCallID: pc.Call.ID, ToolName: pc.Call.Name,
	})
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
