package runtime

import (
	"encoding/json"
	"time"
)

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
	EventTurnCanceled     EventType = "turn.canceled"
	EventRealtimeOpened   EventType = "realtime.opened"
	EventRealtimeClosed   EventType = "realtime.closed"
	EventAudioDelta       EventType = "audio.delta"
	EventTranscriptDelta  EventType = "transcript.delta"
	EventAudioInterrupted EventType = "audio.interrupted"
)

// Event 是两种通道共用的平台事件；Data 是 JSON，不对 JSON 字节重复编码。
type Event struct {
	ID             string          `json:"id"`
	Type           EventType       `json:"type"`
	RequestID      string          `json:"request_id"`
	ConversationID string          `json:"conversation_id"`
	TurnID         string          `json:"turn_id,omitempty"`
	Sequence       int64           `json:"sequence"`
	Data           json.RawMessage `json:"data"`
	OccurredAt     time.Time       `json:"occurred_at"`
}
