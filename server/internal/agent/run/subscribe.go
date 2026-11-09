package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// StreamOptions 控制重放与实时订阅行为。
type StreamOptions struct {
	AfterSeq       int64
	HeartbeatEvery time.Duration
}

// Subscribe 从 PG 重放后接 Redis 或 PG 轮询，按 seq 去重推送。
func Subscribe(ctx context.Context, store Store, redis redisclient.Cmdable, redisOK func() bool, runID string, opts StreamOptions, deliver func(StoredEvent) error) error {
	if opts.HeartbeatEvery <= 0 {
		opts.HeartbeatEvery = 15 * time.Second
	}
	after := opts.AfterSeq
	seen := map[int64]bool{}
	replay, err := store.ListRunEventsAfter(ctx, runID, after)
	if err != nil {
		return err
	}
	for _, ev := range replay {
		if seen[ev.Seq] {
			continue
		}
		seen[ev.Seq] = true
		after = ev.Seq
		if err := deliver(ev); err != nil {
			if errors.Is(err, ErrSubscribeDone) {
				return nil
			}
			return err
		}
		if IsTerminalEvent(ev.Type) {
			return nil
		}
	}
	ok := redisOK
	if ok == nil {
		ok = func() bool { return redis != nil }
	}
	if !ok() || redis == nil {
		return pollPG(ctx, store, runID, &after, seen, deliver)
	}
	key := "xa:run:" + runID + ":events"
	ticker := time.NewTicker(opts.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		default:
		}
		res, err := redis.XRead(ctx, &redisclient.XReadArgs{
			Streams: []string{key, fmt.Sprintf("0-%d", after)},
			Count:   64,
			Block:   15 * time.Second,
		}).Result()
		if err != nil {
			if err == redisclient.Nil {
				continue
			}
			// Redis 故障时降级 PG 轮询。
			return pollPG(ctx, store, runID, &after, seen, deliver)
		}
		for _, stream := range res {
			for _, msg := range stream.Messages {
				seq, ev, err := decodeStreamMessage(msg)
				if err != nil || seen[seq] {
					continue
				}
				seen[seq] = true
				if seq > after {
					after = seq
				}
				if err := deliver(ev); err != nil {
					if errors.Is(err, ErrSubscribeDone) {
						return nil
					}
					return err
				}
				if IsTerminalEvent(ev.Type) {
					return nil
				}
			}
		}
	}
}

func pollPG(ctx context.Context, store Store, runID string, after *int64, seen map[int64]bool, deliver func(StoredEvent) error) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			events, err := store.ListRunEventsAfter(ctx, runID, *after)
			if err != nil {
				return err
			}
			for _, ev := range events {
				if seen[ev.Seq] {
					continue
				}
				seen[ev.Seq] = true
				if ev.Seq > *after {
					*after = ev.Seq
				}
				if err := deliver(ev); err != nil {
					if errors.Is(err, ErrSubscribeDone) {
						return nil
					}
					return err
				}
				if IsTerminalEvent(ev.Type) {
					return nil
				}
			}
		}
	}
}

func decodeStreamMessage(msg redisclient.XMessage) (int64, StoredEvent, error) {
	raw, ok := msg.Values["e"].(string)
	if !ok {
		return 0, StoredEvent{}, fmt.Errorf("流消息缺少字段 e")
	}
	var env struct {
		Seq  int64  `json:"seq"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return 0, StoredEvent{}, err
	}
	return env.Seq, StoredEvent{Seq: env.Seq, Type: env.Type, Payload: json.RawMessage(raw)}, nil
}
