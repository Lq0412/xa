package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// ListRuns 共享 owner 与管理路径，管理路径的角色校验由查询服务执行。
func (h *RunHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			frameworkhttp.WriteError(w, r, 400, "invalid_limit", fmt.Errorf("limit 无效"))
			return
		}
		limit = n
	}
	page, err := h.Runs.ListRuns(r.Context(), p, run.RunFilter{ConversationID: query.Get("conversation_id"), AgentID: query.Get("agent_id"), Status: query.Get("status"), UserID: query.Get("user_id"), Cursor: query.Get("cursor"), Limit: limit}, isAdminRunPath(r))
	if err != nil {
		if errors.Is(err, definition.ErrInvalid) {
			frameworkhttp.WriteError(w, r, 400, "invalid_request", fmt.Errorf("运行筛选参数无效"))
		} else {
			conversationError(w, r, err)
		}
		return
	}
	frameworkhttp.WriteJSON(w, 200, page)
}

// GetRun 返回本次运行摘要和允许出站的配置。
func (h *RunHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	value, err := h.Runs.QueryRun(r.Context(), p, r.PathValue("run_id"), isAdminRunPath(r))
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, value)
}

// Spans 返回脱敏执行树；历史未追踪运行的 span 为 null。
func (h *RunHandler) Spans(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	tree, err := h.Runs.QuerySpans(r.Context(), p, r.PathValue("run_id"), isAdminRunPath(r))
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, tree)
}

// ToolResult 只注册 owner 路径，完整载荷不提供管理员入口。
func (h *RunHandler) ToolResult(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	value, err := h.Runs.QueryToolResult(r.Context(), p, r.PathValue("run_id"), r.PathValue("call_id"))
	if err != nil {
		conversationError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, value)
}

func isAdminRunPath(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/api/v1/admin/runs") }
