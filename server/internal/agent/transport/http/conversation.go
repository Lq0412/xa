package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"net/http"
)

type ConversationHandler struct{ Conversations *conversation.Service }

func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var in struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		frameworkhttp.WriteError(w, r, 400, "invalid_json", err)
		return
	}
	c, err := h.Conversations.Create(r.Context(), p, in.AgentID)
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 201, c)
}
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	v, err := h.Conversations.List(r.Context(), p)
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"conversations": v})
}
func (h *ConversationHandler) Messages(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	v, err := h.Conversations.Messages(r.Context(), p, r.PathValue("conversation_id"))
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"messages": v})
}
func (h *ConversationHandler) Send(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		frameworkhttp.WriteError(w, r, 400, "invalid_json", err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		frameworkhttp.WriteError(w, r, 500, "stream_unsupported", fmt.Errorf("不支持流式响应"))
		return
	}
	emit := func(d string) error {
		b, _ := json.Marshal(map[string]string{"delta": d})
		if _, err := fmt.Fprintf(w, "event: delta\ndata: %s\n\n", b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	m, u, err := h.Conversations.Send(r.Context(), p, r.PathValue("conversation_id"), in.Content, emit)
	if err != nil {
		b, _ := json.Marshal(map[string]string{"message": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
		flusher.Flush()
		return
	}
	b, _ := json.Marshal(map[string]any{"message_id": m.ID, "prompt_tokens": u.PromptTokens, "completion_tokens": u.CompletionTokens})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", b)
	flusher.Flush()
}
func conversationError(w http.ResponseWriter, r *http.Request, err error) {
	status := 500
	code := "conversation_unavailable"
	if errors.Is(err, auth.ErrUnauthenticated) {
		status = 401
		code = "unauthenticated"
	}
	frameworkhttp.WriteError(w, r, status, code, err)
}
