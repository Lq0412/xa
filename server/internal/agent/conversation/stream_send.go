package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/framework/auth"
)

// StreamSendOptions 控制流式响应。
type StreamSendOptions struct {
	Protocol       int
	IdempotencyKey string
	AfterSeq       int64
	WriteV2        func(run.StoredEvent) error
	WriteV1Delta   func(string) error
	WriteV1Done    func(messageID string, prompt, completion int) error
	WriteV1Error   func(msg string) error
}

// EnsureNoActiveRun 在开启 SSE 前拒绝并发写者。
func (s *Service) EnsureNoActiveRun(ctx context.Context, p auth.Principal, conversationID string) error {
	active, err := run.NewPGStore(s.DB).ActiveRunForConversation(ctx, conversationID, p.UserID)
	if err != nil {
		return err
	}
	if active != nil {
		if active.Status == "waiting_approval" {
			return &WaitingApprovalError{RunID: active.ID}
		}
		return fmt.Errorf("对话正在生成回复: %w", definition.ErrConflict)
	}
	return nil
}

// WaitingApprovalError 允许 HTTP 在 SSE 开始前给出等待状态与原 run id。
type WaitingApprovalError struct{ RunID string }

func (*WaitingApprovalError) Error() string {
	return "请先确认上方的操作，或停止本轮后再发送。"
}

// CheckIdempotencyKey 在写 SSE 前校验幂等键是否与正文一致。
func (s *Service) CheckIdempotencyKey(ctx context.Context, conversationID, content, key string, protocol int) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if len(key) > 128 {
		return fmt.Errorf("幂等键过长: %w", definition.ErrInvalid)
	}
	ref, err := run.NewPGStore(s.DB).LookupIdempotency(ctx, conversationID, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if ref.RequestHash != hashRequest(content, protocol) {
		return fmt.Errorf("幂等键已被用于不同内容: %w", definition.ErrInvalid)
	}
	return nil
}

// SendStream 启动异步 run 并订阅 SSE 事件。
func (s *Service) SendStream(ctx context.Context, p auth.Principal, conversationID, content string, opts StreamSendOptions) error {
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("消息不能为空")
	}
	protocol := opts.Protocol
	if protocol <= 0 {
		protocol = 2
	}
	reqHash := hashRequest(content, protocol)

	if opts.IdempotencyKey == "" {
		if active, err := run.NewPGStore(s.DB).ActiveRunForConversation(ctx, conversationID, p.UserID); err != nil {
			return err
		} else if active != nil {
			if active.Status == "waiting_approval" {
				return &WaitingApprovalError{RunID: active.ID}
			}
			return fmt.Errorf("对话正在生成回复: %w", definition.ErrConflict)
		}
	}

	// 幂等：已有 run 则只订阅重放，不新建用户消息或 run。
	if opts.IdempotencyKey != "" {
		if len(opts.IdempotencyKey) > 128 {
			return fmt.Errorf("幂等键过长: %w", definition.ErrInvalid)
		}
		pgStore := run.NewPGStore(s.DB)
		ref, err := pgStore.LookupIdempotency(ctx, conversationID, opts.IdempotencyKey)
		if err == nil {
			if ref.RequestHash != reqHash {
				return fmt.Errorf("幂等键已被用于不同内容: %w", definition.ErrInvalid)
			}
			return s.subscribeRunEvents(ctx, ref.RunID, opts)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, conversationID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("对话正在生成回复: %w", definition.ErrConflict)
	}
	prep, err := s.prepareSend(ctx, tx, p, conversationID, content, protocol)
	if err != nil {
		return err
	}
	runID := "run_" + uuid.NewString()
	userID := "msg_" + uuid.NewString()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user',$3,'completed',$4)`, userID, conversationID, content, now); err != nil {
		return err
	}
	prep.Start.UserMessageID = userID
	runRow := run.Run{
		ID: runID, WorkspaceID: p.WorkspaceID, OwnerUserID: p.UserID, ConversationID: conversationID,
		AgentID: prep.Agent.ID, AgentVersionID: prep.Version.ID, RootRunID: runID,
		UserMessageID: userID, Provider: s.Runs.Provider.Name(), Model: prep.Version.Configuration.Model,
		StreamProtocol: protocol, Status: "running", Limits: prep.Start.Limits, Toolset: prep.Start.Toolset, StartedAt: now,
	}
	// 追踪从上下文组装开始计时，摘要总耗时与执行树根节点使用同一起点。
	if !prep.Start.ContextStartedAt.IsZero() {
		runRow.StartedAt = prep.Start.ContextStartedAt
	}
	if err := run.InsertRunRow(ctx, tx, s.Runs.Store, runRow, run.RunContextSnapshot{
		RunID: runID, Messages: prep.Start.InitialMessages, Segments: prep.Segments,
	}, opts.IdempotencyKey, reqHash, s.Runs.InstanceID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			// 并发幂等或活跃 run 冲突：尝试附着幂等键。
			if opts.IdempotencyKey != "" {
				ref, lerr := run.NewPGStore(s.DB).LookupIdempotency(ctx, conversationID, opts.IdempotencyKey)
				if lerr == nil && ref.RequestHash == reqHash {
					_ = tx.Rollback(ctx)
					return s.subscribeRunEvents(ctx, ref.RunID, opts)
				}
			}
			return fmt.Errorf("对话正在生成回复: %w", definition.ErrConflict)
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if prep.Conversation.Title == "" {
		rn := []rune(content)
		if len(rn) > 30 {
			rn = rn[:30]
		}
		_, _ = s.DB.Exec(ctx, `UPDATE conversations SET title=$1,updated_at=now() WHERE id=$2`, string(rn), conversationID)
	}
	if s.Memory != nil && prep.Policy.Enabled {
		enqueueTx, err := s.DB.Begin(ctx)
		if err == nil {
			if err := s.Memory.EnqueueTx(ctx, enqueueTx, userID); err == nil {
				_ = enqueueTx.Commit(ctx)
			} else {
				_ = enqueueTx.Rollback(ctx)
			}
		}
	}

	bus := run.NewEventBus(runID, s.Runs.Redis, s.Runs.RedisOK, s.Runs.Store)
	var v1 *run.Emitter
	if protocol < 2 && opts.WriteV1Delta != nil {
		v1 = run.NewV1Emitter(opts.WriteV1Delta)
	}
	streamIn := run.StreamStartInput{
		ChatStartInput: prep.Start, RunID: runID, StreamProtocol: protocol,
		IdempotencyKey: opts.IdempotencyKey, RequestHash: reqHash,
		EnableThinking: definition.ChatEnableThinking(prep.Version.Configuration),
	}
	driveCtx := context.WithoutCancel(ctx)
	if protocol < 2 {
		driveCtx = ctx
	}
	s.Runs.StartDrive(driveCtx, streamIn, bus, v1)

	return s.subscribeRunEvents(ctx, runID, opts)
}

func hashRequest(content string, protocol int) string {
	sum := sha256.Sum256([]byte(content + "|" + strconv.Itoa(protocol)))
	return hex.EncodeToString(sum[:])
}

func intFromJSON(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// SubscribeRun 只订阅已有 run（重连）。
func (s *Service) SubscribeRun(ctx context.Context, p auth.Principal, runID string, opts StreamSendOptions) error {
	var owner string
	if err := s.DB.QueryRow(ctx, `SELECT owner_user_id FROM runs WHERE id=$1`, runID).Scan(&owner); err != nil {
		return definition.ErrNotFound
	}
	if owner != p.UserID {
		return definition.ErrNotFound
	}
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
	}
	err := s.subscribeRunEvents(ctx, runID, opts)
	if err != nil && !errors.Is(err, run.ErrSubscribeDone) {
		return err
	}
	return nil
}
