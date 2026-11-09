package run

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"linq-bs/server/internal/framework/redact"
)

func TestMarshalEnvelopeUnknownFieldsForwardCompat(t *testing.T) {
	raw, err := MarshalEnvelope(3, "run_x", EventToolCalled, time.Now().UTC(), 1, map[string]any{
		"call_id": "call_1", "name": "memory_search", "extra_future": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["seq"] != float64(3) || m["type"] != EventToolCalled {
		t.Fatalf("信封字段缺失: %v", m)
	}
	if m["extra_future"] != float64(1) {
		t.Fatal("应保留未知扩展字段")
	}
}

func TestRedactInToolEventPreview(t *testing.T) {
	secret := "Bearer sk-test-not-real-key-abcdef"
	preview := redact.String(`{"query":"` + secret + `"}`)
	if strings.Contains(preview, "sk-test") {
		t.Fatal("args_preview 应脱敏密钥")
	}
}

func TestEventBusDeltaCoalesce(t *testing.T) {
	mem := NewMemStore()
	bus := NewEventBus("run_t", nil, func() bool { return false }, mem)
	bus.SetStepProvider(func() int { return 1 })
	for _, ch := range []string{"a", "b", "c"} {
		if err := bus.MessageDelta(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
	}
	bus.Close()
	events, err := mem.ListRunEventsAfter(context.Background(), "run_t", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("应有合并后的 delta 事件")
	}
	var payload map[string]any
	if err := json.Unmarshal(events[len(events)-1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["delta"] != "abc" {
		t.Fatalf("期望合并 abc，得 %v", payload["delta"])
	}
}
