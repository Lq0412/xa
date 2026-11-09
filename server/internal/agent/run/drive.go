package run

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

// StreamStartInput 启动流式 run 所需的上下文。
type StreamStartInput struct {
	ChatStartInput
	RunID          string
	StreamProtocol int
	IdempotencyKey string
	RequestHash    string
	EnableThinking bool
}

// DriveRun 在独立上下文中执行引擎并发出终止事件。
func (s *RunService) DriveRun(parent context.Context, in StreamStartInput, bus *EventBus, v1 *Emitter) error {
	if s.Coord == nil {
		s.Coord = NewCoordinator()
	}
	runCtx, cancel := context.WithCancelCause(parent)
	s.Coord.Register(in.RunID, cancel, bus)
	defer s.Coord.Finish(in.RunID)

	sink := NewCombinedSink(v1, bus)
	if v1 == nil {
		sink = &CombinedSink{Bus: bus}
	}
	engine := s.Engine
	if engine == nil {
		engine = &NativeEngine{}
	}
	runID := in.RunID
	rc := &RunContext{
		Run: Run{
			ID: runID, WorkspaceID: in.Principal.WorkspaceID, OwnerUserID: in.Principal.UserID,
			ConversationID: in.Conversation.ID, AgentID: in.Agent.ID, AgentVersionID: in.Version.ID,
			RootRunID: runID, UserMessageID: in.UserMessageID, Provider: s.Provider.Name(),
			Model: in.Version.Configuration.Model, StreamProtocol: in.StreamProtocol, Status: "running",
			Limits: in.Limits, Toolset: in.Toolset, StartedAt: time.Now().UTC(),
		},
		Toolset: in.Toolset, Messages: append([]provider.Message(nil), in.InitialMessages...),
		Budget: NewBudget(in.Limits), Store: s.Store, Provider: s.Provider, Registry: s.Registry,
		Env: tool.Env{
			Principal: in.Principal, WorkspaceID: in.Principal.WorkspaceID, AgentID: in.Agent.ID,
			AgentVersionID: in.Version.ID, IsPrimaryAgent: in.Agent.IsPrimary,
			ConversationID: in.Conversation.ID, RunID: runID, UserMessageID: in.UserMessageID,
		},
		Limits: in.Limits, Emitter: sink, Bus: bus, EnableThinking: in.EnableThinking,
		WallStarted: time.Now().UTC(), CallHashes: map[string]int{}, PerToolCalls: map[string]int{},
	}
	bus.SetStepProvider(func() int { return rc.Step })

	if s.Redis != nil {
		_ = acquireConvLock(runCtx, s.Redis, in.Conversation.ID, runID, s.leaseTTL())
	}
	stopLease := s.heartbeatLease(runCtx, runID)
	defer stopLease()

	emitRunStarted(runCtx, rc, in)
	stopWatch := s.startRunWatchers(runCtx, runID, in.StreamProtocol, cancel)
	defer stopWatch()

	began := time.Now()
	outcome, execErr := engine.Execute(runCtx, rc)
	latency := time.Since(began).Milliseconds()
	return s.finalizeStreamingRun(runCtx, rc, outcome, execErr, latency, in)
}

func (s *RunService) leaseTTL() time.Duration {
	if s.Config.LeaseTTLMS > 0 {
		return time.Duration(s.Config.LeaseTTLMS) * time.Millisecond
	}
	return 30 * time.Second
}

func emitRunStarted(ctx context.Context, rc *RunContext, in StreamStartInput) {
	if rc.Emitter == nil {
		return
	}
	tools := make([]map[string]string, 0, len(in.Toolset.Entries))
	for _, e := range in.Toolset.Entries {
		tools = append(tools, map[string]string{"name": e.Spec.Name, "title": e.Spec.Title})
	}
	_ = rc.Emitter.Emit(ctx, EventRunStarted, map[string]any{
		"conversation_id": in.Conversation.ID, "user_message_id": in.UserMessageID,
		"agent_id": in.Agent.ID, "model": in.Version.Configuration.Model,
		"limits": in.Limits, "tools": tools,
		"thinking": map[string]any{"enabled": in.EnableThinking, "budget_tokens": nil},
	})
}

func (s *RunService) finalizeStreamingRun(ctx context.Context, rc *RunContext, outcome Outcome, execErr error, latency int64, in StreamStartInput) error {
	ctx = context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	usage := provider.Usage{PromptTokens: rc.Run.PromptTokens, CompletionTokens: rc.Run.CompletionTokens}
	visible := rc.VisibleText.String()
	if visible == "" && outcome.Kind == OutcomeCompleted {
		visible = "（无回复内容）"
	}
	errText := ""
	assistantStatus := "completed"
	run := rc.Run
	switch outcome.Kind {
	case OutcomeBudgetExhausted:
		run.Status = "budget_exhausted"
		assistantStatus = "failed"
		errText = outcome.Message
		if errText == "" {
			errText = "已达到本轮资源上限"
		}
	case OutcomeCancelled:
		run.Status = "cancelled"
		assistantStatus = "failed"
		errText = "已停止生成"
	case OutcomeFailed:
		run.Status = "failed"
		assistantStatus = "failed"
		if outcome.Message != "" {
			errText = outcome.Message
		} else {
			errText = "生成失败"
		}
	default:
		run.Status = "completed"
	}
	if execErr == nil && visible != "" && outcome.Kind != OutcomeBudgetExhausted && outcome.Kind != OutcomeCancelled {
		run.Status = "completed"
		assistantStatus = "completed"
		errText = ""
	}
	run.Steps = rc.Step
	run.ModelTurns = rc.Budget.Turns
	run.ToolCalls = rc.Budget.ToolCalls
	run.LatencyMS = latency
	run.ActiveMS = ActiveMS(rc.WallStarted)
	if outcome.Code != "" {
		run.ErrorCode = outcome.Code
	}
	ended := time.Now().UTC()
	run.EndedAt = &ended

	tx, err := s.Store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	assistantID := "msg_" + uuid.NewString()
	segments := in.Segments
	if len(segments) == 0 {
		segments = []byte("[]")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO messages(id,conversation_id,role,content,model,prompt_tokens,completion_tokens,latency_ms,status,error,created_at,context_segments,agent_version_id,run_id)
VALUES($1,$2,'assistant',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		assistantID, in.Conversation.ID, visible, in.Version.Configuration.Model,
		usage.PromptTokens, usage.CompletionTokens, latency, assistantStatus, errText,
		ended, segments, in.Version.ID, in.RunID); err != nil {
		return err
	}
	run.FinalMessageID = assistantID
	if err := s.Store.SaveRunContext(ctx, tx, RunContextSnapshot{
		RunID: in.RunID, Messages: rc.Messages, Segments: in.Segments,
	}); err != nil {
		return err
	}
	if err := s.Store.UpdateRun(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	termPayload := terminalPayload(assistantID, run, outcome, errText, usage, latency, rc)
	typ := EventRunCompleted
	if outcome.Kind == OutcomeCancelled {
		typ = EventRunCancelled
	} else if outcome.Kind == OutcomeFailed || outcome.Kind == OutcomeBudgetExhausted {
		typ = EventRunFailed
	}
	if rc.Emitter != nil {
		_ = rc.Emitter.Emit(ctx, typ, termPayload)
	}
	if rc.Bus != nil {
		rc.Bus.Close()
	}
	releaseConvLock(ctx, s.Redis, in.Conversation.ID, in.RunID)
	if execErr != nil && outcome.Kind == OutcomeFailed {
		return execErr
	}
	return nil
}

func terminalPayload(assistantID string, run Run, outcome Outcome, errText string, usage provider.Usage, latency int64, rc *RunContext) map[string]any {
	if outcome.Kind == OutcomeCancelled {
		code := outcome.Code
		if code == "" {
			code = "user_cancel"
		}
		return map[string]any{"message_id": assistantID, "status": "cancelled", "code": code, "message": errText}
	}
	if outcome.Kind == OutcomeFailed || outcome.Kind == OutcomeBudgetExhausted {
		return map[string]any{
			"message_id": assistantID, "status": string(outcome.Kind), "code": run.ErrorCode, "message": errText,
		}
	}
	return map[string]any{
		"message_id": assistantID, "status": "completed", "truncated": outcome.Truncated,
		"usage": map[string]any{
			"prompt": usage.PromptTokens, "completion": usage.CompletionTokens,
			"reasoning": 0, "cached": usage.CachedTokens, "total": usage.TotalTokens,
		},
		"model_turns": rc.Budget.Turns, "tool_calls": rc.Budget.ToolCalls, "latency_ms": latency,
	}
}

func (s *RunService) startRunWatchers(ctx context.Context, runID string, streamProtocol int, cancel context.CancelCauseFunc) func() {
	stop := make(chan struct{})
	grace := time.Duration(s.Config.ReconnectGraceSec) * time.Second
	if streamProtocol < 2 {
		grace = 0
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var missingSince time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if s.Redis == nil {
					continue
				}
				if cancelRequested(ctx, s.Redis, runID) {
					cancel(errors.New("user_cancel"))
					return
				}
				if grace == 0 {
					continue
				}
				if HasPresence(ctx, s.Redis, runID) {
					missingSince = time.Time{}
					continue
				}
				if missingSince.IsZero() {
					missingSince = time.Now()
					continue
				}
				if time.Since(missingSince) > grace {
					cancel(errors.New("client_disconnected"))
					return
				}
			}
		}
	}()
	return func() { close(stop) }
}

func cancelRequested(ctx context.Context, r redisclient.Cmdable, runID string) bool {
	if r == nil {
		return false
	}
	v, err := r.Get(ctx, "xa:run:"+runID+":cancel").Result()
	return err == nil && v != ""
}

func (s *RunService) heartbeatLease(ctx context.Context, runID string) func() {
	if s.DB == nil {
		return func() {}
	}
	owner := s.InstanceID
	if owner == "" {
		owner = "local"
	}
	ttl := s.leaseTTL()
	renew := func() {
		_, _ = s.DB.Exec(ctx, `UPDATE runs SET lease_owner=$2,lease_expires_at=now()+($3::text||' milliseconds')::interval,heartbeat_at=now() WHERE id=$1`,
			runID, owner, int(ttl.Milliseconds()))
	}
	renew()
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(ttl / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				renew()
			}
		}
	}()
	return func() { close(stop) }
}
