package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/framework/workspace"
)

// ManagementHandler 暴露 Agent 配置管理接口；调用者身份始终从认证上下文读取。
type ManagementHandler struct{ Agents definition.Service }

type agentRequest struct {
	AgentID           string                   `json:"agent_id"`
	ExpectedVersionID string                   `json:"expected_version_id"`
	Name              string                   `json:"name"`
	Avatar            string                   `json:"avatar"`
	Description       string                   `json:"description"`
	Tags              []string                 `json:"tags"`
	Visibility        definition.Visibility    `json:"visibility"`
	Configuration     definition.Configuration `json:"configuration"`
}

func (h *ManagementHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := frameworkhttp.Principal(r.Context())
	if !ok {
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
		return
	}
	list, err := h.Agents.List(r.Context(), principal)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, map[string]any{"agents": list})
}

func (h *ManagementHandler) Available(w http.ResponseWriter, r *http.Request) {
	principal, ok := frameworkhttp.Principal(r.Context())
	if !ok {
		principal = auth.Principal{WorkspaceID: workspace.DefaultWorkspaceID}
	}
	list, err := h.Agents.List(r.Context(), principal)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	public := make([]definition.AgentDefinition, 0, len(list))
	for _, agent := range list {
		if agent.Visibility == definition.VisibilityPublic && agent.Status == definition.StatusActive {
			public = append(public, agent)
		}
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, map[string]any{"agents": public})
}

func (h *ManagementHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var input agentRequest
	if !decodeAgentJSON(w, r, &input) {
		return
	}
	input.AgentID = ""
	saved, err := h.Agents.Save(r.Context(), principal, toSaveInput(input))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusCreated, agentResponse(saved))
}

func (h *ManagementHandler) Get(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	agent, err := h.Agents.Get(r.Context(), principal, r.PathValue("agent_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, agent)
}

func (h *ManagementHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	agent, err := h.Agents.Get(r.Context(), principal, r.PathValue("agent_id"))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	version, err := h.Agents.GetVersion(r.Context(), principal, agent.ID, agent.CurrentVersionID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, version)
}

func (h *ManagementHandler) Save(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var input agentRequest
	if !decodeAgentJSON(w, r, &input) {
		return
	}
	input.AgentID = r.PathValue("agent_id")
	saved, err := h.Agents.Save(r.Context(), principal, toSaveInput(input))
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, agentResponse(saved))
}

func (h *ManagementHandler) Enable(w http.ResponseWriter, r *http.Request)  { h.setStatus(w, r, true) }
func (h *ManagementHandler) Disable(w http.ResponseWriter, r *http.Request) { h.setStatus(w, r, false) }

func (h *ManagementHandler) setStatus(w http.ResponseWriter, r *http.Request, enabled bool) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var err error
	if enabled {
		err = h.Agents.Enable(r.Context(), principal, r.PathValue("agent_id"))
	} else {
		err = h.Agents.Disable(r.Context(), principal, r.PathValue("agent_id"))
	}
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ManagementHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := h.Agents.Delete(r.Context(), principal, r.PathValue("agent_id")); err != nil {
		writeAgentError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requirePrincipal(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	principal, ok := frameworkhttp.Principal(r.Context())
	if !ok || principal.UserID == "" {
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
		return auth.Principal{}, false
	}
	return principal, true
}

func toSaveInput(input agentRequest) definition.SaveInput {
	return definition.SaveInput{AgentID: input.AgentID, ExpectedVersionID: input.ExpectedVersionID, Name: input.Name, Avatar: input.Avatar, Description: input.Description, Tags: input.Tags, Visibility: input.Visibility, Configuration: input.Configuration}
}

func agentResponse(saved definition.Saved) map[string]any {
	return map[string]any{"agent": saved.Definition, "current_version_id": saved.Version.ID, "version_number": saved.Version.VersionNumber, "config_checksum": saved.Version.ConfigChecksum}
}

func decodeAgentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		frameworkhttp.WriteError(w, r, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

func writeAgentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", err)
	case errors.Is(err, definition.ErrNotFound):
		frameworkhttp.WriteError(w, r, http.StatusNotFound, "agent_not_found", err)
	case errors.Is(err, definition.ErrForbidden):
		frameworkhttp.WriteError(w, r, http.StatusForbidden, "agent_forbidden", err)
	case errors.Is(err, definition.ErrConflict):
		frameworkhttp.WriteError(w, r, http.StatusConflict, "agent_conflict", err)
	case errors.Is(err, definition.ErrInvalid):
		frameworkhttp.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_agent", err)
	default:
		frameworkhttp.WriteError(w, r, http.StatusServiceUnavailable, "agent_unavailable", err)
	}
}
