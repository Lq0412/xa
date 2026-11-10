package http

import (
	"encoding/json"
	"errors"
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
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
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
	if h.Runs == nil {
		frameworkhttp.WriteError(w, r, 500, "run_unavailable", fmt.Errorf("RunService 未配置"))
		return
	}
	status, err := h.Runs.Cancel(r.Context(), p, runID)
	if err != nil {
		var ae *run.ApprovalError
		if errors.As(err, &ae) {
			frameworkhttp.WriteError(w, r, ae.Status, ae.Code, err)
			return
		}
		conversationError(w, r, err)
		return
	}
	code := 200
	if status == "cancelling" {
		code = 202
	}
	frameworkhttp.WriteJSON(w, code, map[string]string{"status": status})
}

// Decision 只接受 owner 本人的审批决定。
func (h *RunHandler) Decision(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var in run.DecisionInput
	if !decodeAgentJSON(w, r, &in) {
		return
	}
	result, err := h.Runs.Decide(r.Context(), p, r.PathValue("approval_id"), in)
	if err != nil {
		var ae *run.ApprovalError
		if errors.As(err, &ae) {
			frameworkhttp.WriteError(w, r, ae.Status, ae.Code, err)
			return
		}
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, result)
}
