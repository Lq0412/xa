package conversation

import (
	"encoding/json"
	"testing"
	"time"

	"linq-bs/server/internal/agent/run"
)

func TestBuildPartsFromEventsToolAndText(t *testing.T) {
	payload := func(typ string, extra map[string]any) []byte {
		m := map[string]any{"type": typ, "run_id": "run_t", "seq": 1}
		for k, v := range extra {
			m[k] = v
		}
		b, _ := json.Marshal(m)
		return b
	}
	events := []replayEvent{
		{Type: "tool.preparing", Payload: payload("tool.preparing", map[string]any{"name": "current_time", "title": "当前时间", "index": 0})},
		{Type: "tool.called", Payload: payload("tool.called", map[string]any{"call_id": "c1", "name": "current_time", "title": "当前时间", "index": 0, "status": "pending"})},
		{Type: "tool.started", Payload: payload("tool.started", map[string]any{"call_id": "c1", "name": "current_time", "title": "当前时间"})},
		{Type: "tool.completed", Payload: payload("tool.completed", map[string]any{"call_id": "c1", "name": "current_time", "title": "当前时间", "status": "succeeded", "summary": "ok"})},
		{Type: "message.delta", Payload: payload("message.delta", map[string]any{"delta": "现在是"})},
	}
	parts := buildPartsFromEvents(events, partsReplayOpts{stripReasoningText: true})
	if len(parts) < 2 {
		t.Fatalf("parts=%v", parts)
	}
	if parts[0]["type"] != "tool" || parts[0]["status"] != "succeeded" {
		t.Fatalf("tool part=%v", parts[0])
	}
	last := parts[len(parts)-1]
	if last["type"] != "text" || last["text"] != "现在是" {
		t.Fatalf("text=%v", last)
	}
}

func TestBuildPartsReasoningOrderWithLateCompleted(t *testing.T) {
	payload := func(seq int64, typ string, extra map[string]any) []byte {
		m := map[string]any{"type": typ, "run_id": "run_t", "seq": seq}
		for k, v := range extra {
			m[k] = v
		}
		b, _ := json.Marshal(m)
		return b
	}
	events := []replayEvent{
		{Type: run.EventReasoningDelta, Payload: payload(1, run.EventReasoningDelta, map[string]any{"delta": "x", "item_id": "r1"})},
		{Type: run.EventMessageDelta, Payload: payload(2, run.EventMessageDelta, map[string]any{"delta": "正文"})},
		{Type: run.EventReasoningCompleted, Payload: payload(3, run.EventReasoningCompleted, map[string]any{
			"item_id": "r1", "duration_ms": 1200,
		})},
	}
	parts := buildPartsFromEvents(events, partsReplayOpts{stripReasoningText: true})
	if len(parts) != 2 {
		t.Fatalf("parts=%v", parts)
	}
	if parts[0]["type"] != "reasoning" {
		t.Fatalf("first part should be reasoning, got %v", parts[0])
	}
	if parts[1]["type"] != "text" || parts[1]["text"] != "正文" {
		t.Fatalf("text part=%v", parts[1])
	}
	if parts[0]["duration_ms"] != 1200 {
		t.Fatalf("duration_ms=%v", parts[0]["duration_ms"])
	}
}

func TestBuildPartsReasoningStripped(t *testing.T) {
	now := time.Now().UTC()
	raw, _ := run.MarshalEnvelope(1, "run_t", run.EventReasoningDelta, now, 1, map[string]any{"delta": "secret", "item_id": "r1"})
	completed, _ := run.MarshalEnvelope(2, "run_t", run.EventReasoningCompleted, now, 1, map[string]any{"item_id": "r1"})
	parts := buildPartsFromEvents([]replayEvent{
		{Type: run.EventReasoningDelta, Payload: raw},
		{Type: run.EventReasoningCompleted, Payload: completed},
	}, partsReplayOpts{stripReasoningText: true})
	found := false
	for _, p := range parts {
		if p["type"] == "reasoning" {
			found = true
			if p["text"] != "" {
				t.Fatalf("reasoning text should be stripped")
			}
			if p["status"] != "completed" {
				t.Fatalf("status=%v", p["status"])
			}
		}
	}
	if !found {
		t.Fatal("missing reasoning stub")
	}
}
