package http

import (
	"encoding/json"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"net/http"
	"strconv"
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
	// 旧客户端不传分页参数，继续返回完整列表及原有优先级顺序。
	if !q.Has("limit") && !q.Has("cursor") && q.Get("scope") != "all_visible" {
		items, err := h.Memory.List(r.Context(), p, memory.Filter{Scope: q.Get("scope"), AgentID: q.Get("agent_id"), Namespace: q.Get("namespace"), Status: q.Get("status")})
		if err != nil {
			writeAgentError(w, r, err)
			return
		}
		frameworkhttp.WriteJSON(w, 200, memory.Page{Items: items})
		return
	}
	limit := 50
	if q.Get("limit") != "" {
		var err error
		if limit, err = strconv.Atoi(q.Get("limit")); err != nil {
			writeAgentError(w, r, definition.ErrInvalid)
			return
		}
	}
	if limit > 200 {
		limit = 200
	}
	page, err := h.Memory.ListPage(r.Context(), p, memory.Filter{Scope: q.Get("scope"), AgentID: q.Get("agent_id"), Namespace: q.Get("namespace"), Status: q.Get("status")}, q.Get("cursor"), limit)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, page)
}

// Add 显式新增当前用户的手动记忆。
func (h *MemoryHandler) Add(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var in memory.AddInput
	if !decodeAgentJSON(w, r, &in) {
		return
	}
	item, err := h.Memory.Add(r.Context(), p, in)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 201, item)
}

// Restore 返回保留原条目历史的新版本。
func (h *MemoryHandler) Restore(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	item, err := h.Memory.Restore(r.Context(), p, r.PathValue("memory_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, item)
}

// Stats 仅统计当前用户的条目和任务。
func (h *MemoryHandler) Stats(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	v, err := h.Memory.Stats(r.Context(), p)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, v)
}

// Retry 将所有者的 dead 任务重新入队。
func (h *MemoryHandler) Retry(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := h.Memory.RetryJob(r.Context(), p, r.PathValue("job_id")); err != nil {
		writeAgentError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		ExpectedVersionID    string   `json:"expected_version_id"`
		Enabled              *bool    `json:"enabled"`
		CoreBudgetTokens     *int     `json:"core_budget_tokens"`
		RecallTopK           *int     `json:"recall_top_k"`
		MinRelevance         *float64 `json:"min_relevance"`
		HistoryBudgetTokens  *int     `json:"history_budget_tokens"`
		SummaryTriggerTokens *int     `json:"summary_trigger_tokens"`
		SummaryBudgetTokens  *int     `json:"summary_budget_tokens"`
		DecayEnabled         *bool    `json:"decay_enabled"`
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
	if input.MinRelevance != nil {
		policy.MinRelevance = *input.MinRelevance
	}
	if input.HistoryBudgetTokens != nil {
		policy.HistoryBudgetTokens = *input.HistoryBudgetTokens
	}
	if input.SummaryTriggerTokens != nil {
		policy.SummaryTriggerTokens = *input.SummaryTriggerTokens
	}
	if input.SummaryBudgetTokens != nil {
		policy.SummaryBudgetTokens = *input.SummaryBudgetTokens
	}
	if input.DecayEnabled != nil {
		policy.DecayEnabled = *input.DecayEnabled
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
