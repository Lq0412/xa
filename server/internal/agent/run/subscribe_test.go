package run

import (
	"context"
	"testing"
	"time"
)

func TestSubscribeReplayDedup(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	runID := "run_sub"
	bus := NewEventBus(runID, nil, func() bool { return false }, store)
	for i := 0; i < 3; i++ {
		_ = bus.Emit(ctx, EventMessageDelta, map[string]any{"delta": "a"})
	}
	_ = bus.Emit(ctx, EventRunCompleted, map[string]any{"status": "completed"})
	bus.Close()
	events, err := store.ListRunEventsAfter(ctx, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("应有写入 PG 的事件")
	}
	seen := map[int64]bool{}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = Subscribe(ctx, store, nil, func() bool { return false }, runID, StreamOptions{}, func(ev StoredEvent) error {
		if seen[ev.Seq] {
			t.Fatal("重复事件")
		}
		seen[ev.Seq] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
