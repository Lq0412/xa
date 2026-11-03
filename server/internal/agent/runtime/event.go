package runtime

import "time"

// EventType 是对外暴露的运行事件类型。
type EventType string

const (
	EventTurnStarted      EventType = "turn.started"
	EventMessageDelta     EventType = "message.delta"
	EventToolStarted      EventType = "tool.started"
	EventToolCompleted    EventType = "tool.completed"
	EventMessageCompleted EventType = "message.completed"
	EventTurnCompleted    EventType = "turn.completed"
	EventTurnFailed       EventType = "turn.failed"
)

// Event 是 Eino 事件转换后的平台事件。
type Event struct {
	Type           EventType
	RequestID      string
	ConversationID string
	TurnID         string
	Sequence       int64
	Data           []byte
	OccurredAt     time.Time
}
