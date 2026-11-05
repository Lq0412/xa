package http

import (
	"net/http"

	"linq-bs/server/internal/agent/definition"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// LibraryHandler 只管理当前登录用户的公共 Agent 添加记录。
type LibraryHandler struct{ Library *definition.Library }

// List 返回对话侧栏所需的真实 Agent 集合。
func (h *LibraryHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	agents, err := h.Library.List(r.Context(), principal)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// Add 不接受客户端传入用户 ID，归属由认证上下文确定。
func (h *LibraryHandler) Add(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := h.Library.Add(r.Context(), principal, r.PathValue("agent_id")); err != nil {
		writeAgentError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
