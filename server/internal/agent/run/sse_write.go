package run

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteSSEEnvelope 按 v2 协议写出单条事件。
func WriteSSEEnvelope(w http.ResponseWriter, flusher http.Flusher, ev StoredEvent) error {
	var env map[string]any
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return err
	}
	typ, _ := env["type"].(string)
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, typ, ev.Payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// WriteSSEPing 发送心跳。
func WriteSSEPing(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// V1Project 将 v2 信封投影为 v1 SSE。
func V1Project(ev StoredEvent, emit func(string) error, onDone func(map[string]any) error, onError func(string) error) (terminal bool, err error) {
	var env map[string]any
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return false, err
	}
	typ, _ := env["type"].(string)
	switch typ {
	case EventMessageDelta:
		d, _ := env["delta"].(string)
		return false, emit(d)
	case EventRunCompleted:
		if onDone != nil {
			return true, onDone(env)
		}
		return true, nil
	case EventRunFailed, EventRunCancelled:
		msg, _ := env["message"].(string)
		if onError != nil {
			return true, onError(msg)
		}
		return true, nil
	default:
		return false, nil
	}
}
