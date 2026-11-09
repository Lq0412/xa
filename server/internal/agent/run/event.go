package run

import (
	"encoding/json"
	"fmt"
	"time"
)

// 事件类型常量；审批与 tool.output 本期仅预留类型名，供前端忽略未知 type。
const (
	EventRunStarted         = "run.started"
	EventStepStarted        = "step.started"
	EventReasoningDelta     = "reasoning.delta"
	EventReasoningCompleted = "reasoning.completed"
	EventMessageDelta       = "message.delta"
	EventToolPreparing      = "tool.preparing"
	EventToolCalled         = "tool.called"
	EventToolStarted        = "tool.started"
	EventToolOutput         = "tool.output"
	EventToolCompleted      = "tool.completed"
	EventApprovalRequired   = "approval.required"
	EventRunWaiting         = "run.waiting"
	EventApprovalResolved   = "approval.resolved"
	EventMemoryUpdated      = "memory.updated"
	EventStepCompleted      = "step.completed"
	EventRunUsage           = "run.usage"
	EventRunCompleted       = "run.completed"
	EventRunFailed          = "run.failed"
	EventRunCancelled       = "run.cancelled"
)

var terminalEventTypes = map[string]bool{
	EventRunCompleted: true,
	EventRunFailed:    true,
	EventRunCancelled: true,
}

// IsTerminalEvent 判断是否为终止类事件。
func IsTerminalEvent(typ string) bool { return terminalEventTypes[typ] }

// Envelope 是 SSE v2 与 run_events.payload 的完整信封。
type Envelope struct {
	Seq   int64          `json:"seq"`
	RunID string         `json:"run_id"`
	Type  string         `json:"type"`
	TS    string         `json:"ts"`
	Step  int            `json:"step,omitempty"`
	Extra map[string]any `json:"-"`
}

// MarshalEnvelope 将公共字段与类型 payload 平铺为 JSON。
func MarshalEnvelope(seq int64, runID, typ string, at time.Time, step int, payload map[string]any) ([]byte, error) {
	m := map[string]any{
		"seq":    seq,
		"run_id": runID,
		"type":   typ,
		"ts":     formatEventTime(at),
	}
	if step > 0 {
		m["step"] = step
	}
	for k, v := range payload {
		if k == "seq" || k == "run_id" || k == "type" || k == "ts" || k == "step" {
			continue
		}
		m[k] = v
	}
	return json.Marshal(m)
}

var shanghaiLoc = time.FixedZone("Asia/Shanghai", 8*60*60)

func formatEventTime(t time.Time) string {
	tt := t.UTC().In(shanghaiLoc)
	ms := tt.Nanosecond() / 1e6
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d.%03d+08:00",
		tt.Year(), tt.Month(), tt.Day(), tt.Hour(), tt.Minute(), tt.Second(), ms)
}
