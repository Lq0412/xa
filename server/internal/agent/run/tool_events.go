package run

import (
	"context"
	"encoding/json"

	"linq-bs/server/internal/framework/redact"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

func emitToolPreparing(ctx context.Context, rc *RunContext, index int, name, title string) {
	if rc.Emitter == nil {
		return
	}
	_ = rc.Emitter.Emit(ctx, EventToolPreparing, map[string]any{"index": index, "name": name, "title": title})
}

func emitToolCalled(ctx context.Context, rc *RunContext, pc PlannedCall) {
	if rc.Emitter == nil {
		return
	}
	entry, ok := rc.Toolset.EntryByName(pc.Call.Name)
	title := pc.Call.Name
	if ok {
		title = entry.Spec.Title
	}
	status := "pending"
	payload := map[string]any{
		"call_id": pc.Call.ID, "index": pc.Index, "name": pc.Call.Name, "title": title, "status": status,
		"args_preview": truncateRunes(redact.String(pc.Call.Arguments), 512),
	}
	if !pc.Accepted {
		payload["status"] = "rejected"
		payload["error_code"] = pc.ErrorCode
	}
	_ = rc.Emitter.Emit(ctx, EventToolCalled, payload)
}

func emitToolStarted(ctx context.Context, rc *RunContext, callID, name, title string) {
	if rc.Emitter == nil {
		return
	}
	_ = rc.Emitter.Emit(ctx, EventToolStarted, map[string]any{"call_id": callID, "name": name, "title": title})
}

func emitToolCompleted(ctx context.Context, rc *RunContext, row ToolResultRow, entry tool.Entry, durationMS int64) {
	if rc.Emitter == nil {
		return
	}
	title := row.ToolName
	if entry.Spec.Title != "" {
		title = entry.Spec.Title
	}
	summary, items := toolEventSummary(row)
	payload := map[string]any{
		"call_id": row.CallID, "name": row.ToolName, "title": title, "status": row.Status,
		"summary": redact.String(summary), "duration_ms": durationMS, "truncated": row.Truncated, "result_ref": row.ID,
	}
	if row.ErrorCode != "" {
		payload["error_code"] = row.ErrorCode
	}
	if len(items) > 0 {
		payload["items"] = items
	}
	_ = rc.Emitter.Emit(ctx, EventToolCompleted, payload)
}

// toolEventSummary 优先用展示摘要；失败且没有摘要时带上错误信息，供时间线直接显示。
func toolEventSummary(row ToolResultRow) (string, []map[string]string) {
	summary, items := displayPreview(row.Display)
	if summary == "" {
		summary = row.ErrorMessage
	}
	return summary, items
}

func displayPreview(raw json.RawMessage) (string, []map[string]string) {
	if len(raw) == 0 {
		return "", nil
	}
	var d tool.Display
	if err := json.Unmarshal(raw, &d); err != nil {
		return "", nil
	}
	items := make([]map[string]string, 0, len(d.Items))
	for _, it := range d.Items {
		items = append(items, map[string]string{"label": it.Label, "value": redact.String(it.Value)})
	}
	return d.Summary, items
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// streamSink 将模型流事件转为 SSE 生命周期事件。
func streamSink(ctx context.Context, rc *RunContext) func(provider.StreamEvent) error {
	return func(ev provider.StreamEvent) error {
		switch ev.Type {
		case provider.EventTextDelta:
			if ev.Text == "" {
				return nil
			}
			rc.Emitted = true
			rc.VisibleText.WriteString(ev.Text)
			if rc.Emitter != nil {
				return rc.Emitter.TextDelta(ev.Text)
			}
		case provider.EventReasoningDelta:
			if ev.Reasoning == "" || rc.Emitter == nil {
				return nil
			}
			step := rc.Step
			if step <= 0 {
				step = 1
			}
			itemID := itemIDOr("", step)
			if rc.Bus != nil {
				return rc.Bus.ReasoningDelta(ctx, itemID, ev.Reasoning)
			}
			return rc.Emitter.Emit(ctx, EventReasoningDelta, map[string]any{"delta": redact.String(ev.Reasoning), "item_id": itemID})
		case provider.EventToolCallStart:
			entry, ok := rc.Toolset.EntryByName(ev.ToolName)
			title := ev.ToolName
			if ok {
				title = entry.Spec.Title
			}
			emitToolPreparing(ctx, rc, ev.Index, ev.ToolName, title)
		}
		return nil
	}
}
