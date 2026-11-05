package http

import (
	"encoding/json"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"net/http"
)

// MemoryHandler 所有操作从登录会话确定所有者，不接受 user_id。
type MemoryHandler struct {
	Memory *memory.Service
	Agents definition.Service
}

// List 返回当前用户的记忆条目。
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := h.Memory.List(r.Context(), p, memory.Filter{Scope: q.Get("scope"), AgentID: q.Get("agent_id"), Namespace: q.Get("namespace"), Status: q.Get("status")})
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"items": items})
}

// Core 返回当前用户在指定 Agent 中可读的 memory.md。
func (h *MemoryHandler) Core(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	view, err := h.Memory.Core(r.Context(), p, r.PathValue("agent_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, view)
}

// Edit 使用完整编辑字段创建取代条目。
func (h *MemoryHandler) Edit(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var input memory.EditInput
	if !decodeAgentJSON(w, r, &input) {
		return
	}
	item, err := h.Memory.Edit(r.Context(), p, r.PathValue("memory_id"), input)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, item)
}

// Delete 仅允许记忆所有者删除。
func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := h.Memory.Delete(r.Context(), p, r.PathValue("memory_id")); err != nil {
		writeAgentError(w, r, err)
		return
	}
	w.WriteHeader(204)
}

// Events 返回包含前后版本的取代链审计。
func (h *MemoryHandler) Events(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	events, err := h.Memory.Events(r.Context(), p, r.PathValue("memory_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"events": events})
}

// Policy 保留其他配置并生成新版本，必须提供期望版本防止覆盖并发编辑。
func (h *MemoryHandler) Policy(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersionID string `json:"expected_version_id"`
		Enabled           *bool  `json:"enabled"`
		CoreBudgetTokens  *int   `json:"core_budget_tokens"`
		RecallTopK        *int   `json:"recall_top_k"`
	}
	if !decodeAgentJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		writeAgentError(w, r, definition.ErrInvalid)
		return
	}
	a, err := h.Agents.Get(r.Context(), p, r.PathValue("agent_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	if a.OwnerUserID != p.UserID {
		writeAgentError(w, r, definition.ErrForbidden)
		return
	}
	v, err := h.Agents.GetVersion(r.Context(), p, a.ID, a.CurrentVersionID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	policy, err := definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	policy.Enabled = *input.Enabled
	if input.CoreBudgetTokens != nil {
		policy.CoreBudgetTokens = *input.CoreBudgetTokens
	}
	if input.RecallTopK != nil {
		policy.RecallTopK = *input.RecallTopK
	}
	v.Configuration.MemoryPolicy, err = json.Marshal(policy)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	saved, err := h.Agents.Save(r.Context(), p, definition.SaveInput{AgentID: a.ID, ExpectedVersionID: input.ExpectedVersionID, Name: a.Name, Avatar: a.Avatar, Description: a.Description, Tags: a.Tags, Visibility: a.Visibility, Configuration: v.Configuration})
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"current_version_id": saved.Version.ID, "version_number": saved.Version.VersionNumber, "memory_policy": policy})
}
