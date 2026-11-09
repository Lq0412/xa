package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/framework/auth"
)

// StreamSendOptions 控制流式响应。
type StreamSendOptions struct {
	Protocol     int
	AfterSeq     int64
	WriteV2      func(run.StoredEvent) error
	WriteV1Delta func(string) error
	WriteV1Done  func(messageID string, prompt, completion int) error
	WriteV1Error func(msg string) error
	OnPresence   func()
}

// SendStream 启动异步 run 并订阅 SSE 事件。
func (s *Service) SendStream(ctx context.Context, p auth.Principal, conversationID, content string, opts StreamSendOptions) error {
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
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
	prep, err := s.prepareSend(ctx, tx, p, conversationID, content)
	if err != nil {
		return err
	}
	protocol := opts.Protocol
	if protocol <= 0 {
		protocol = 2
	}
	runID := "run_" + uuid.NewString()
	userID := "msg_" + uuid.NewString()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user',$3,'completed',$4)`, userID, conversationID, content, now); err != nil {
		return err
	}
	prep.Start.UserMessageID = userID
	reqHash := hashRequest(content, protocol)
	runRow := run.Run{
		ID: runID, WorkspaceID: p.WorkspaceID, OwnerUserID: p.UserID, ConversationID: conversationID,
		AgentID: prep.Agent.ID, AgentVersionID: prep.Version.ID, RootRunID: runID,
		UserMessageID: userID, Provider: s.Runs.Provider.Name(), Model: prep.Version.Configuration.Model,
		StreamProtocol: protocol, Status: "running", Limits: prep.Start.Limits, Toolset: prep.Start.Toolset, StartedAt: now,
	}
	if err := run.InsertRunRow(ctx, tx, s.Runs.Store, runRow, run.RunContextSnapshot{
		RunID: runID, Messages: prep.Start.InitialMessages, Segments: prep.Segments,
	}, "", reqHash, s.Runs.InstanceID); err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
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
		RequestHash: reqHash, EnableThinking: definition.ChatEnableThinking(prep.Version.Configuration),
	}
	driveCtx := context.WithoutCancel(ctx)
	if protocol < 2 {
		driveCtx = ctx
	}
	s.Runs.StartDrive(driveCtx, streamIn, bus, v1)

	presence := time.NewTicker(5 * time.Second)
	defer presence.Stop()
	return run.Subscribe(ctx, s.Runs.Store, s.Runs.Redis, s.Runs.RedisOK, runID, run.StreamOptions{AfterSeq: opts.AfterSeq, HeartbeatEvery: 15 * time.Second}, func(ev run.StoredEvent) error {
		select {
		case <-presence.C:
			run.TouchPresence(ctx, s.Runs.Redis, runID, "main")
			if opts.OnPresence != nil {
				opts.OnPresence()
			}
		default:
		}
		if protocol >= 2 && opts.WriteV2 != nil {
			if err := opts.WriteV2(ev); err != nil {
				return err
			}
		} else {
			term, err := run.V1Project(ev, opts.WriteV1Delta, func(m map[string]any) error {
				mid, _ := m["message_id"].(string)
				u, _ := m["usage"].(map[string]any)
				if opts.WriteV1Done != nil {
					return opts.WriteV1Done(mid, intFromJSON(u["prompt"]), intFromJSON(u["completion"]))
				}
				return nil
			}, func(msg string) error {
				if opts.WriteV1Error != nil {
					return opts.WriteV1Error(msg)
				}
				return nil
			})
			if err != nil {
				return err
			}
			if term {
				return run.ErrSubscribeDone
			}
		}
		var env map[string]any
		_ = json.Unmarshal(ev.Payload, &env)
		typ, _ := env["type"].(string)
		if run.IsTerminalEvent(typ) {
			if protocol >= 2 && (typ == run.EventRunFailed || typ == run.EventRunCancelled) {
				msg, _ := env["message"].(string)
				if msg != "" {
					return fmt.Errorf("%s", msg)
				}
			}
			return run.ErrSubscribeDone
		}
		return nil
	})
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
	var owner, conv string
	if err := s.DB.QueryRow(ctx, `SELECT owner_user_id,conversation_id FROM runs WHERE id=$1`, runID).Scan(&owner, &conv); err != nil {
		return definition.ErrNotFound
	}
	if owner != p.UserID {
		return definition.ErrNotFound
	}
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
	}
	presence := time.NewTicker(5 * time.Second)
	defer presence.Stop()
	err := run.Subscribe(ctx, s.Runs.Store, s.Runs.Redis, s.Runs.RedisOK, runID, run.StreamOptions{AfterSeq: opts.AfterSeq, HeartbeatEvery: 15 * time.Second}, func(ev run.StoredEvent) error {
		select {
		case <-presence.C:
			run.TouchPresence(ctx, s.Runs.Redis, runID, "resume")
		default:
		}
		if opts.WriteV2 != nil {
			return opts.WriteV2(ev)
		}
		var env map[string]any
		_ = json.Unmarshal(ev.Payload, &env)
		if run.IsTerminalEvent(env["type"].(string)) {
			return run.ErrSubscribeDone
		}
		return nil
	})
	if err != nil && !errors.Is(err, run.ErrSubscribeDone) {
		return err
	}
	return nil
}
