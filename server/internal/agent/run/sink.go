package run

import (
	"context"
)

// EventSink 是引擎向 SSE 投影与事件总线输出的统一入口。
type EventSink interface {
	PendingSeparator(sep string)
	TextDelta(delta string) error
	Emit(ctx context.Context, typ string, payload map[string]any) error
}

// CombinedSink 将 v1 文本投影与 v2 事件总线组合。
type CombinedSink struct {
	V1  *Emitter
	Bus *EventBus
}

// NewCombinedSink 绑定 v1 回调与 v2 总线。
func NewCombinedSink(v1 *Emitter, bus *EventBus) *CombinedSink {
	return &CombinedSink{V1: v1, Bus: bus}
}

func (c *CombinedSink) PendingSeparator(sep string) {
	if c.V1 != nil {
		c.V1.PendingSeparator(sep)
	}
}

func (c *CombinedSink) TextDelta(delta string) error {
	if c.Bus != nil {
		if err := c.Bus.MessageDelta(context.Background(), delta); err != nil {
			return err
		}
	}
	if c.V1 != nil {
		return c.V1.TextDelta(delta)
	}
	return nil
}

func (c *CombinedSink) Emit(ctx context.Context, typ string, payload map[string]any) error {
	if c.Bus == nil {
		return nil
	}
	return c.Bus.Emit(ctx, typ, payload)
}

// V1Sink 仅包装旧 v1 Emitter，供不启用总线的测试。
type V1Sink struct{ E *Emitter }

func (v *V1Sink) PendingSeparator(sep string) {
	if v.E != nil {
		v.E.PendingSeparator(sep)
	}
}

func (v *V1Sink) TextDelta(delta string) error {
	if v.E == nil {
		return nil
	}
	return v.E.TextDelta(delta)
}

func (v *V1Sink) Emit(context.Context, string, map[string]any) error { return nil }
