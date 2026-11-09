package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"linq-bs/server/internal/agent/run"
)

// subscribeRunEvents 附着到已有 run 的事件流（幂等重试 / 重连）。
func (s *Service) subscribeRunEvents(ctx context.Context, runID string, opts StreamSendOptions) error {
	if s.Runs == nil {
		return fmt.Errorf("RunService 未配置")
	}
	presence := time.NewTicker(5 * time.Second)
	defer presence.Stop()
	return run.Subscribe(ctx, s.Runs.Store, s.Runs.Redis, s.Runs.RedisOK, runID, run.StreamOptions{
		AfterSeq: opts.AfterSeq, HeartbeatEvery: 15 * time.Second,
	}, func(ev run.StoredEvent) error {
		select {
		case <-presence.C:
			run.TouchPresence(ctx, s.Runs.Redis, runID, "attach")
			if opts.OnPresence != nil {
				opts.OnPresence()
			}
		default:
		}
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
