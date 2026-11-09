package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/run"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// RunHandler 处理 run 事件流与取消。
type RunHandler struct {
	Conversations *conversation.Service
	Runs          *run.RunService
}

func (h *RunHandler) Events(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	runID := r.PathValue("run_id")
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			frameworkhttp.WriteError(w, r, http.StatusBadRequest, "invalid_after", fmt.Errorf("after 无效"))
			return
		}
		after = v
	}
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > after {
			after = v
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		frameworkhttp.WriteError(w, r, 500, "stream_unsupported", fmt.Errorf("不支持流式响应"))
		return
	}
	err := h.Conversations.SubscribeRun(r.Context(), p, runID, conversation.StreamSendOptions{
		Protocol: 2, AfterSeq: after,
		WriteV2: func(ev run.StoredEvent) error {
			return run.WriteSSEEnvelope(w, flusher, ev)
		},
	})
	if err != nil {
		b, _ := json.Marshal(map[string]string{"message": conversation.PublicError(err)})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
		flusher.Flush()
	}
}

func (h *RunHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	runID := r.PathValue("run_id")
	var owner string
	if err := h.Conversations.DB.QueryRow(r.Context(), `SELECT owner_user_id FROM runs WHERE id=$1`, runID).Scan(&owner); err != nil {
		conversationError(w, r, fmt.Errorf("not found"))
		return
	}
	if owner != p.UserID {
		conversationError(w, r, fmt.Errorf("not found"))
		return
	}
	if h.Runs == nil {
		frameworkhttp.WriteError(w, r, 500, "run_unavailable", fmt.Errorf("RunService 未配置"))
		return
	}
	_ = h.Runs.Coord
	if h.Runs.Coord == nil {
		h.Runs.Coord = run.NewCoordinator()
	}
	_ = h.Runs.Coord.CancelRun(r.Context(), h.Runs.Redis, runID, "user_cancel")
	frameworkhttp.WriteJSON(w, 202, map[string]string{"status": "cancelling"})
}
