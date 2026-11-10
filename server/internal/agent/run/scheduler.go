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
	Suspended bool
}

// Scheduler 按写屏障分批，在审批调用前持久化全部待执行调用。
type Scheduler struct{}

// Run 执行本步被接受的调用。
func (s Scheduler) Run(ctx context.Context, rc *RunContext, accepted []PlannedCall) (SchedulerOutput, error) {
	if len(accepted) == 0 {
		return SchedulerOutput{}, nil
	}
	if err := persistPendingCalls(ctx, rc, accepted); err != nil {
		return SchedulerOutput{}, err
	}
	return s.runPending(ctx, rc, accepted)
}

// runPending 执行已落盘的调用，续跑时不重复写入之前的结果。
func (Scheduler) runPending(ctx context.Context, rc *RunContext, accepted []PlannedCall) (SchedulerOutput, error) {
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
			pc := batch.calls[0]
			suspended, err := approvalGate(ctx, rc, pc)
			if err != nil {
				return out, err
			}
			if suspended {
				out.Suspended = true
				return out, nil
			}
			row, err := rc.toolResult(ctx, pc.Call.ID)
			if err != nil {
				return out, err
			}
			if row.Status == "rejected" {
				out.AllFailed = true
				continue
			}
			if err := executeOne(ctx, rc, pc); err != nil {
				out.Cancelled = ctx.Err() != nil
				return out, err
			}
		}
	}
	return out, nil
}

func persistPendingCalls(ctx context.Context, rc *RunContext, calls []PlannedCall) error {
	return rc.withToolTx(ctx, func(tx pgx.Tx) error {
		for _, pc := range calls {
			entry, _ := rc.Toolset.EntryByName(pc.Call.Name)
			args := []byte(pc.Call.Arguments)
			if len(args) == 0 {
				args = []byte("{}")
			}
			row := ToolResultRow{ID: NewResultRef(), RunID: rc.Run.ID, ConversationID: rc.Env.ConversationID,
				OwnerUserID: rc.Env.Principal.UserID, Step: rc.Step, CallIndex: pc.Index + 1, CallID: pc.Call.ID,
				ToolName: pc.Call.Name, ToolVersion: entry.Spec.Version, Effect: string(entry.Spec.Effect),
				Arguments: args, RawArguments: string(args), ArgsHash: argsHash(pc.Call.Name, args), Status: "pending"}
			if err := rc.Store.InsertToolResult(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
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
	// 同一 pgx.Tx 不能跨 goroutine 并发使用；Chat 事务路径退化为串行仍保留模型侧 parallel_tool_calls。
	if rc.Tx != nil {
		for _, pc := range calls {
			if ctx.Err() != nil {
				return failed, ctx.Err()
			}
			if e := executeOne(ctx, rc, pc); e != nil {
				if ctx.Err() != nil {
					return failed + 1, e
				}
				failed++
				err = e
			}
		}
		return failed, err
	}
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
	rc.mu.Lock()
	rc.PerToolCalls[pc.Call.Name]++
	rc.mu.Unlock()
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
	row, err := rc.toolResult(ctx, pc.Call.ID)
	if err != nil {
		return err
	}
	resultRef := row.ID
	row.Status, row.Attempts = "running", 1
	if err := rc.withToolTx(ctx, func(tx pgx.Tx) error {
		return rc.Store.UpdateToolResult(ctx, tx, row)
	}); err != nil {
		return err
	}
	emitToolStarted(ctx, rc, pc.Call.ID, pc.Call.Name, entry.Spec.Title)
	started := time.Now()

	toolCtx, cancel := context.WithTimeout(ctx, minDuration(entry.Timeout, rc.RemainingWall()))
	defer cancel()

	var res tool.Result
	var execErr error
	if txTool, ok := toolImpl.(tool.TxTool); ok {
		prepared, prepErr := txTool.Prepare(toolCtx, call, env)
		if prepErr != nil {
			execErr = prepErr
		} else if rc.Tx != nil {
			env.Tx = rc.Tx
			res, execErr = txTool.ExecuteTx(toolCtx, rc.Tx, call, env, prepared)
			if execErr == nil {
				rc.dbMu.Lock()
				execErr = persistToolSuccess(ctx, rc, rc.Tx, &row, res, entry, resultRef)
				rc.dbMu.Unlock()
			}
			env.Tx = nil
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
				execErr = tx2.Commit(ctx)
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
			if rc.Tx != nil {
				rc.dbMu.Lock()
				execErr = persistToolSuccess(ctx, rc, rc.Tx, &row, res, entry, resultRef)
				rc.dbMu.Unlock()
			} else {
				execErr = rc.withToolTx(ctx, func(tx pgx.Tx) error {
					return persistToolSuccess(ctx, rc, tx, &row, res, entry, resultRef)
				})
			}
		}
	}
	if execErr != nil {
		err := persistToolFailure(ctx, rc, &row, execErr)
		emitToolCompleted(ctx, rc, row, entry, time.Since(started).Milliseconds())
		return err
	}
	emitToolCompleted(ctx, rc, row, entry, time.Since(started).Milliseconds())
	if rc.Emitter != nil {
		for _, effect := range res.Effects {
			if effect.Kind == "memory.delete" || effect.Kind == "memory.write" || effect.Kind == "memory.add" || effect.Kind == "memory.update" {
				if err := rc.Emitter.Emit(ctx, EventMemoryUpdated, map[string]any{"kind": effect.Kind, "memory_id": effect.TargetID}); err != nil {
					return err
				}
			}
		}
	}
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
	if err := rc.withToolTx(ctx, func(tx pgx.Tx) error {
		return rc.Store.UpdateToolResult(ctx, tx, *row)
	}); err != nil {
		return err
	}
	rc.appendMessage(toolMessage(row.ToolName, row.CallID, content))
	return rc.appendRunMessage(ctx, RunMessage{
		RunID: rc.Run.ID, Step: rc.Step, Role: "tool", Content: content,
		ToolCallID: row.CallID, ToolName: row.ToolName,
	})
}

func appendToolMessage(ctx context.Context, rc *RunContext, pc PlannedCall, content string) error {
	rc.appendMessage(toolMessage(pc.Call.Name, pc.Call.ID, content))
	return rc.appendRunMessage(ctx, RunMessage{
		RunID: rc.Run.ID, Step: rc.Step, Role: "tool", Content: content,
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
