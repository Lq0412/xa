package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// EventRecorder 将事件写入 PG 与 Redis。
type EventRecorder interface {
	AppendRunEvent(ctx context.Context, runID string, seq int64, typ string, payload []byte, terminal bool) error
	UpdateLastEventSeq(ctx context.Context, runID string, seq int64) error
	ListRunEventsAfter(ctx context.Context, runID string, afterSeq int64) ([]StoredEvent, error)
}

// StoredEvent 是从 PG 读回的 run_events 行。
type StoredEvent struct {
	Seq     int64
	Type    string
	Payload json.RawMessage
}

// EventBus 分配 seq、合并 delta、发布到 Redis 与 PG。
type EventBus struct {
	RunID    string
	Redis    redisclient.Cmdable
	RedisOK  func() bool
	Recorder EventRecorder

	mu                 sync.Mutex
	seq                int64
	stepFn             func() int
	msgBuf             strings.Builder
	reasonBuf          strings.Builder
	reasonItemID       string
	reasoningOpen      bool
	reasoningStartedAt time.Time
	flushTimer         *time.Timer
	pendingFlush       bool
	batch              []pendingEvent
	terminals          []pendingEvent
	lastFlushedSeq     int64
	subscribers        map[int]chan StoredEvent
	nextSubID          int
	closed             bool
	deltaCoalesceMS    int
	deltaCoalesceLen   int
}

type pendingEvent struct {
	seq      int64
	typ      string
	payload  []byte
	at       time.Time
	step     int
	terminal bool
}

// NewEventBus 创建事件总线；redis 可为 nil（仅 PG）。
func NewEventBus(runID string, redis redisclient.Cmdable, redisOK func() bool, rec EventRecorder) *EventBus {
	ok := redisOK
	if ok == nil {
		ok = func() bool { return redis != nil }
	}
	return &EventBus{
		RunID: runID, Redis: redis, RedisOK: ok, Recorder: rec,
		deltaCoalesceMS: 50, deltaCoalesceLen: 256,
		subscribers: map[int]chan StoredEvent{},
		stepFn:      func() int { return 0 },
	}
}

// SetStepProvider 提供当前模型 step，供信封 step 字段使用。
func (b *EventBus) SetStepProvider(fn func() int) { b.stepFn = fn }

// Close 刷写剩余缓冲并关闭订阅通道。
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}
	b.flushDeltasLocked()
	b.flushBatchLocked(true)
	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = nil
}

func (b *EventBus) currentStep() int {
	if b.stepFn != nil {
		return b.stepFn()
	}
	return 0
}

// Emit 发布非 delta 类事件；终止事件立即落盘。
func (b *EventBus) Emit(ctx context.Context, typ string, payload map[string]any) error {
	if typ == EventMessageDelta {
		d, _ := payload["delta"].(string)
		return b.MessageDelta(ctx, d)
	}
	if typ == EventReasoningDelta {
		d, _ := payload["delta"].(string)
		id, _ := payload["item_id"].(string)
		return b.ReasoningDelta(ctx, id, d)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("事件总线已关闭")
	}
	b.flushDeltasLocked()
	seq := b.seq + 1
	raw, err := MarshalEnvelope(seq, b.RunID, typ, time.Now().UTC(), b.currentStep(), payload)
	if err != nil {
		return err
	}
	pe := pendingEvent{seq: seq, typ: typ, payload: raw, at: time.Now().UTC(), step: b.currentStep(), terminal: IsTerminalEvent(typ)}
	b.seq = seq
	if pe.terminal {
		b.terminals = append(b.terminals, pe)
		b.flushBatchLocked(true)
		return nil
	}
	b.batch = append(b.batch, pe)
	if strings.HasPrefix(typ, "tool.") {
		b.flushBatchLocked(false)
		return nil
	}
	if len(b.batch) >= 32 {
		b.flushBatchLocked(false)
	} else if b.flushTimer == nil {
		b.flushTimer = time.AfterFunc(500*time.Millisecond, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.closed {
				return
			}
			b.flushDeltasLocked()
			b.flushBatchLocked(false)
		})
	}
	_ = ctx
	return nil
}

// MessageDelta 合并可见正文增量。
func (b *EventBus) MessageDelta(ctx context.Context, delta string) error {
	if delta == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("事件总线已关闭")
	}
	b.msgBuf.WriteString(delta)
	if b.msgBuf.Len() >= b.deltaCoalesceLen {
		b.flushDeltasLocked()
	} else {
		b.scheduleDeltaFlushLocked()
	}
	_ = ctx
	return nil
}

// ReasoningDelta 合并思考增量，不参与 v1 投影。
func (b *EventBus) ReasonDelta(ctx context.Context, itemID, delta string) error {
	return b.ReasoningDelta(ctx, itemID, delta)
}

func (b *EventBus) ReasoningDelta(ctx context.Context, itemID, delta string) error {
	if delta == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("事件总线已关闭")
	}
	if itemID != "" {
		b.reasonItemID = itemID
	}
	b.reasoningOpen = true
	if b.reasonBuf.Len() == 0 {
		b.reasoningStartedAt = time.Now()
	}
	b.reasonBuf.WriteString(delta)
	b.flushDeltasLocked()
	b.flushBatchLocked(false)
	_ = ctx
	return nil
}

// ReasoningCompleted 结束本步思考块。
func (b *EventBus) ReasoningCompleted(ctx context.Context, truncated bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushDeltasLocked()
	chars := b.reasonBuf.Len()
	item := b.reasonItemID
	if item == "" && b.currentStep() > 0 {
		item = fmt.Sprintf("reason_step%d", b.currentStep())
	}
	if !b.reasoningOpen && chars == 0 {
		return nil
	}
	b.reasoningOpen = false
	b.reasonBuf.Reset()
	completedAt := time.Now()
	var durationMS int64
	if !b.reasoningStartedAt.IsZero() {
		durationMS = completedAt.Sub(b.reasoningStartedAt).Milliseconds()
	}
	b.reasoningStartedAt = time.Time{}
	seq := b.seq + 1
	payload := map[string]any{
		"item_id": itemIDOr(item, b.currentStep()), "chars": chars, "truncated": truncated,
	}
	if durationMS > 0 {
		payload["duration_ms"] = durationMS
	}
	raw, err := MarshalEnvelope(seq, b.RunID, EventReasoningCompleted, completedAt.UTC(), b.currentStep(), payload)
	if err != nil {
		return err
	}
	b.seq = seq
	b.batch = append(b.batch, pendingEvent{seq: seq, typ: EventReasoningCompleted, payload: raw, at: time.Now().UTC(), step: b.currentStep()})
	b.flushBatchLocked(false)
	_ = ctx
	return nil
}

func itemIDOr(item string, step int) string {
	if item != "" {
		return item
	}
	return fmt.Sprintf("reason_step%d", step)
}

func (b *EventBus) scheduleDeltaFlushLocked() {
	if b.flushTimer != nil {
		return
	}
	b.flushTimer = time.AfterFunc(time.Duration(b.deltaCoalesceMS)*time.Millisecond, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.closed {
			return
		}
		b.flushDeltasLocked()
	})
}

func (b *EventBus) flushDeltasLocked() {
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}
	if b.msgBuf.Len() > 0 {
		delta := b.msgBuf.String()
		b.msgBuf.Reset()
		seq := b.seq + 1
		raw, _ := MarshalEnvelope(seq, b.RunID, EventMessageDelta, time.Now().UTC(), b.currentStep(), map[string]any{"delta": delta})
		b.seq = seq
		b.batch = append(b.batch, pendingEvent{seq: seq, typ: EventMessageDelta, payload: raw, at: time.Now().UTC(), step: b.currentStep()})
	}
	if b.reasonBuf.Len() > 0 {
		delta := b.reasonBuf.String()
		b.reasonBuf.Reset()
		seq := b.seq + 1
		item := itemIDOr(b.reasonItemID, b.currentStep())
		raw, _ := MarshalEnvelope(seq, b.RunID, EventReasoningDelta, time.Now().UTC(), b.currentStep(), map[string]any{"delta": delta, "item_id": item})
		b.seq = seq
		b.batch = append(b.batch, pendingEvent{seq: seq, typ: EventReasoningDelta, payload: raw, at: time.Now().UTC(), step: b.currentStep()})
	}
}

func (b *EventBus) flushBatchLocked(forceTerminal bool) {
	ctx := context.Background()
	toFlush := append([]pendingEvent{}, b.batch...)
	b.batch = nil
	if forceTerminal {
		toFlush = append(toFlush, b.terminals...)
		b.terminals = nil
	}
	if len(toFlush) == 0 {
		return
	}
	var maxSeq int64
	for _, pe := range toFlush {
		if err := b.persistOne(ctx, pe); err != nil {
			continue
		}
		if pe.seq > maxSeq {
			maxSeq = pe.seq
		}
		b.notifySubscribers(pe)
	}
	if maxSeq > 0 && b.Recorder != nil {
		_ = b.Recorder.UpdateLastEventSeq(ctx, b.RunID, maxSeq)
		b.lastFlushedSeq = maxSeq
	}
	if !forceTerminal && len(b.batch) == 0 && b.flushTimer == nil {
		b.flushTimer = time.AfterFunc(500*time.Millisecond, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.closed {
				return
			}
			b.flushDeltasLocked()
			b.flushBatchLocked(false)
		})
	}
}

func (b *EventBus) persistOne(ctx context.Context, pe pendingEvent) error {
	if b.Recorder != nil {
		if err := b.Recorder.AppendRunEvent(ctx, b.RunID, pe.seq, pe.typ, pe.payload, pe.terminal); err != nil {
			return err
		}
	}
	if b.RedisOK() && b.Redis != nil {
		key := "xa:run:" + b.RunID + ":events"
		_ = b.Redis.XAdd(ctx, &redisclient.XAddArgs{
			Stream: key,
			ID:     fmt.Sprintf("0-%d", pe.seq),
			Values: map[string]any{"e": string(pe.payload)},
		}).Err()
		if pe.terminal {
			_ = b.Redis.Expire(ctx, key, 24*time.Hour).Err()
		}
	}
	return nil
}

func (b *EventBus) notifySubscribers(pe pendingEvent) {
	if len(b.subscribers) == 0 {
		return
	}
	ev := StoredEvent{Seq: pe.seq, Type: pe.typ, Payload: pe.payload}
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

// SubscribeLocal 注册内存订阅，供同进程 SSE 转发。
func (b *EventBus) SubscribeLocal() (<-chan StoredEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextSubID
	b.nextSubID++
	ch := make(chan StoredEvent, 64)
	b.subscribers[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
	}
}
