package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/run"
)

// subscribeRunEvents 附着到已有 run 的事件流（幂等重试 / 重连）。
func (s *Service) subscribeRunEvents(ctx context.Context, runID string, opts StreamSendOptions) error {
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
	}
	// 思考和等待审批时可能没有事件，连接存活标记由独立时钟刷新。
	if opts.Protocol >= 2 && s.Runs.Redis != nil {
		presenceCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		subID := uuid.NewString()
		run.TouchPresence(presenceCtx, s.Runs.Redis, runID, subID)
		go func() {
			defer close(done)
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-presenceCtx.Done():
					return
				case <-ticker.C:
					run.TouchPresence(presenceCtx, s.Runs.Redis, runID, subID)
				}
			}
		}()
		defer func() { cancel(); <-done }()
	}
	return run.Subscribe(ctx, s.Runs.Store, s.Runs.Redis, s.Runs.RedisOK, runID, run.StreamOptions{
		AfterSeq: opts.AfterSeq, HeartbeatEvery: 5 * time.Second,
	}, func(ev run.StoredEvent) error {
		protocol := opts.Protocol
		if protocol <= 0 {
			protocol = 2
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
