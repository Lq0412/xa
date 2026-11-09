package http

import (
	"encoding/json"
	"net/http"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/tool"
)

// ToolsHandler 暴露工具目录与 Agent 工具配置 API。
type ToolsHandler struct {
	Registry *tool.Registry
	Agents   definition.Service
}

// Catalog 返回平台工具目录。
func (h *ToolsHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePrincipal(w, r); !ok {
		return
	}
	if h.Registry == nil {
		frameworkhttp.WriteError(w, r, http.StatusServiceUnavailable, "tool_unavailable", nil)
		return
	}
	items := []map[string]any{}
	for _, spec := range h.Registry.Catalog() {
		items = append(items, catalogItem(spec))
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"tools": items})
}

// GetAgentTools 返回 Agent 当前版本生效的工具视图。
func (h *ToolsHandler) GetAgentTools(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	view, err := h.agentToolsView(r, p, r.PathValue("agent_id"), true)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, 200, view)
}

// PutAgentTools 更新 Agent 工具与 runtime_limits 配置。
func (h *ToolsHandler) PutAgentTools(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersionID string                `json:"expected_version_id"`
		Tools             []binding.ToolBinding `json:"tools"`
		RuntimeLimits     json.RawMessage       `json:"runtime_limits"`
	}
	if !decodeAgentJSON(w, r, &input) {
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
	v.Configuration.Tools = input.Tools
	if len(input.RuntimeLimits) > 0 {
		limits, err := definition.ParseRuntimeLimits(input.RuntimeLimits)
		if err != nil {
			writeAgentError(w, r, err)
			return
		}
		v.Configuration.RuntimeLimits, err = json.Marshal(limits)
		if err != nil {
			writeAgentError(w, r, definition.ErrInvalid)
			return
		}
	}
	saved, err := h.Agents.Save(r.Context(), p, definition.SaveInput{
		AgentID: a.ID, ExpectedVersionID: input.ExpectedVersionID,
		Name: a.Name, Avatar: a.Avatar, Description: a.Description, Tags: a.Tags, Visibility: a.Visibility,
		Configuration: v.Configuration,
	})
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	view, err := h.agentToolsView(r, p, a.ID, false)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	view["current_version_id"] = saved.Version.ID
	view["version_number"] = saved.Version.VersionNumber
	frameworkhttp.WriteJSON(w, 200, view)
}

func (h *ToolsHandler) agentToolsView(r *http.Request, p auth.Principal, agentID string, hideOtherOwners bool) (map[string]any, error) {
	a, err := h.Agents.Get(r.Context(), p, agentID)
	if err != nil {
		return nil, err
	}
	if hideOtherOwners && a.OwnerUserID != p.UserID {
		return nil, definition.ErrForbidden
	}
	v, err := h.Agents.GetVersion(r.Context(), p, a.ID, a.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	policy, err := definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
	if err != nil {
		return nil, err
	}
	limits, err := definition.ParseRuntimeLimits(v.Configuration.RuntimeLimits)
	if err != nil {
		return nil, err
	}
	toolset, err := h.Registry.Resolve(tool.ResolveInput{
		Bindings: v.Configuration.Tools, MemoryPolicy: policy, Principal: p, CanApprove: true,
	})
	if err != nil {
		return nil, err
	}
	tools := []map[string]any{}
	for _, e := range toolset.Entries {
		tools = append(tools, map[string]any{
			"name": e.Spec.Name, "title": e.Spec.Title, "enabled": e.Enabled, "approval": string(e.Approval),
			"timeout_ms": e.Timeout.Milliseconds(), "max_calls": e.MaxCalls, "configuration": jsonOrEmpty(e.Config),
			"available": e.Spec.Available, "reason": e.Reason,
		})
	}
	limitsRaw, err := json.Marshal(limits)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"current_version_id": a.CurrentVersionID,
		"tools":              tools,
		"runtime_limits":     json.RawMessage(limitsRaw),
		"toolset_hash":       toolset.Hash,
	}, nil
}

func catalogItem(spec tool.Spec) map[string]any {
	return map[string]any{
		"name": spec.Name, "title": spec.Title, "description": spec.Description, "effect": spec.Effect,
		"default_enabled": spec.DefaultEnabled,
		"approval":        map[string]any{"default": spec.Approval.Default, "overridable": spec.Approval.Overridable},
		"available":       spec.Available, "config_schema": jsonOrEmpty(spec.ConfigSchema), "input_schema": jsonOrEmpty(spec.InputSchema),
	}
}

func jsonOrEmpty(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	return v
}
