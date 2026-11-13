package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/prompt"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

const agentBodyLimit = 2 << 20

// ManagementHandler 暴露 Agent 配置管理接口，调用者身份来自认证上下文。
type ManagementHandler struct {
	Agents        definition.Service
	AllowedModels []string
}

type agentRequest struct {
	AgentID           string                `json:"agent_id,omitempty" doc:"兼容旧客户端，服务端以请求路径为准"`
	ExpectedVersionID string                `json:"expected_version_id,omitempty" doc:"更新时用于并发校验的当前版本 ID"`
	Name              string                `json:"name" doc:"Agent 名称"`
	Avatar            string                `json:"avatar,omitempty" doc:"头像"`
	Description       string                `json:"description,omitempty" doc:"描述"`
	Tags              []string              `json:"tags,omitempty" doc:"标签"`
	Visibility        definition.Visibility `json:"visibility" doc:"可见范围" enum:"public,private"`
	Configuration     configurationRequest  `json:"configuration,omitempty" doc:"运行配置，省略字段由 Agent 服务处理"`
}

type configurationRequest definition.Configuration

// Schema 复用运行配置的字段类型，允许写入时省略由服务填充的配置。
func (configurationRequest) Schema(registry huma.Registry) *huma.Schema {
	schema := *registry.Schema(reflect.TypeFor[definition.Configuration](), false, "Configuration")
	schema.Required = nil
	return &schema
}

type createAgentInput struct {
	Body agentRequest
}

type agentPath struct {
	AgentID string `path:"agent_id" doc:"Agent ID"`
}

type saveAgentInput struct {
	AgentID string `path:"agent_id" doc:"Agent ID"`
	Body    agentRequest
}

type agentListResponse struct {
	Agents []definition.AgentDefinition `json:"agents" doc:"符合当前访问条件的 Agent"`
}

type agentListOutput struct {
	Body agentListResponse
}

type agentOutput struct {
	Body definition.AgentDefinition
}

type agentConfigResponse struct {
	definition.AgentVersion
	AvailableModels []string `json:"available_models" doc:"可选择的模型" nullable:"false"`
}

type agentConfigOutput struct {
	Body agentConfigResponse
}

type agentSaveResponse struct {
	Agent            definition.AgentDefinition `json:"agent" doc:"保存后的 Agent"`
	CurrentVersionID string                     `json:"current_version_id" doc:"当前配置版本 ID"`
	VersionNumber    int                        `json:"version_number" doc:"配置版本序号"`
	ConfigChecksum   string                     `json:"config_checksum" doc:"配置校验值"`
}

type agentSaveOutput struct {
	Body agentSaveResponse
}

// RegisterRoutes 注册 Agent 管理接口和公开 Agent 列表。
func (h *ManagementHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const tag = "Agent 管理"
	const agents = "/api/v1/agents"
	const agent = agents + "/{agent_id}"
	protect := func(next http.Handler) http.Handler { return frameworkhttp.Authenticate(resolver, next) }
	allowPublic := func(next http.Handler) http.Handler { return frameworkhttp.OptionalPrincipal(resolver, next) }
	descriptions := map[int]string{
		400: "请求 JSON 无效（invalid_json）", 401: "会话无效（unauthenticated）", 403: "没有 Agent 操作权限（agent_forbidden）",
		404: "Agent 或配置版本不存在（agent_not_found）", 409: "配置版本冲突（agent_conflict）", 422: "Agent 信息或配置无效，包括知识库绑定权限、可见性或 filter 不受支持（invalid_agent）",
		503: "认证或 Agent 服务不可用（unauthenticated、agent_unavailable）",
	}
	protected := func(id, method, path, summary string, status int, maxBodyBytes int64, codes ...int) huma.Operation {
		errors := make(map[int]string, len(codes))
		for _, code := range codes {
			errors[code] = descriptions[code]
		}
		return frameworkhttp.OperationAuth(frameworkhttp.Operation(api, tag, id, method, path, summary, status, maxBodyBytes, errors), protect, false)
	}
	available := frameworkhttp.Operation(api, tag, "availableAgents", http.MethodGet, "/api/v1/available-agents", "获取公开可用 Agent", http.StatusOK, 0, map[int]string{
		401: descriptions[401], 503: "Agent 服务不可用（agent_unavailable）",
	})
	huma.Register(api, frameworkhttp.OperationAuth(available, allowPublic, true), h.available)
	huma.Register(api, protected("listAgents", http.MethodGet, agents, "列出 Agent", http.StatusOK, 0, 401, 503), h.list)
	huma.Register(api, protected("createAgent", http.MethodPost, agents, "创建 Agent", http.StatusCreated, agentBodyLimit, 400, 401, 403, 409, 422, 503), h.create)
	huma.Register(api, protected("getAgent", http.MethodGet, agent, "获取 Agent", http.StatusOK, 0, 401, 403, 404, 503), h.get)
	huma.Register(api, protected("getAgentConfig", http.MethodGet, agent+"/config", "获取 Agent 当前配置", http.StatusOK, 0, 401, 403, 404, 503), h.getConfig)
	huma.Register(api, protected("saveAgent", http.MethodPut, agent+"/config", "保存 Agent 配置", http.StatusOK, agentBodyLimit, 400, 401, 403, 404, 409, 422, 503), h.save)
	huma.Register(api, protected("enableAgent", http.MethodPost, agent+"/enable", "启用 Agent", http.StatusNoContent, 0, 401, 403, 404, 503), h.enable)
	huma.Register(api, protected("offlineAgent", http.MethodPost, agent+"/offline", "下线 Agent", http.StatusNoContent, 0, 401, 403, 404, 503), h.offline)
	huma.Register(api, protected("deleteAgent", http.MethodDelete, agent, "删除 Agent", http.StatusNoContent, 0, 401, 403, 404, 503), h.delete)
}

func (h *ManagementHandler) list(ctx context.Context, _ *frameworkhttp.EmptyInput) (*agentListOutput, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	list, err := h.Agents.List(ctx, principal)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &agentListOutput{Body: agentListResponse{Agents: list}}, nil
}

func (h *ManagementHandler) available(ctx context.Context, _ *frameworkhttp.EmptyInput) (*agentListOutput, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	list, err := h.Agents.List(ctx, principal)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	public := make([]definition.AgentDefinition, 0, len(list))
	for _, agent := range list {
		if agent.Visibility == definition.VisibilityPublic && agent.Status == definition.StatusActive {
			public = append(public, agent)
		}
	}
	return &agentListOutput{Body: agentListResponse{Agents: public}}, nil
}

func (h *ManagementHandler) create(ctx context.Context, input *createAgentInput) (*agentSaveOutput, error) {
	return h.saveAgent(ctx, "", input.Body)
}

func (h *ManagementHandler) get(ctx context.Context, input *agentPath) (*agentOutput, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	agent, err := h.Agents.Get(ctx, principal, input.AgentID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &agentOutput{Body: agent}, nil
}

func (h *ManagementHandler) getConfig(ctx context.Context, input *agentPath) (*agentConfigOutput, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	agent, err := h.Agents.Get(ctx, principal, input.AgentID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	version, err := h.Agents.GetVersion(ctx, principal, agent.ID, agent.CurrentVersionID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	// 旧管理端继续通过 prompts 编辑，用户端使用 soul_md。
	version.Configuration.Prompts = []prompt.Slot{{Name: "system", Content: version.Configuration.SoulMD, Enabled: true, Scope: "agent"}}
	return &agentConfigOutput{Body: agentConfigResponse{AgentVersion: version, AvailableModels: h.AllowedModels}}, nil
}

func (h *ManagementHandler) save(ctx context.Context, input *saveAgentInput) (*agentSaveOutput, error) {
	return h.saveAgent(ctx, input.AgentID, input.Body)
}

func (h *ManagementHandler) saveAgent(ctx context.Context, id string, input agentRequest) (*agentSaveOutput, huma.StatusError) {
	principal, _ := frameworkhttp.Principal(ctx)
	saved, err := h.Agents.Save(ctx, principal, definition.SaveInput{
		AgentID: id, ExpectedVersionID: input.ExpectedVersionID, Name: input.Name, Avatar: input.Avatar,
		Description: input.Description, Tags: input.Tags, Visibility: input.Visibility, Configuration: definition.Configuration(input.Configuration),
	})
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &agentSaveOutput{Body: agentSaveResponse{
		Agent: saved.Definition, CurrentVersionID: saved.Version.ID,
		VersionNumber: saved.Version.VersionNumber, ConfigChecksum: saved.Version.ConfigChecksum,
	}}, nil
}

func (h *ManagementHandler) enable(ctx context.Context, input *agentPath) (*frameworkhttp.NoContent, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	if err := h.Agents.Enable(ctx, principal, input.AgentID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}

func (h *ManagementHandler) offline(ctx context.Context, input *agentPath) (*frameworkhttp.NoContent, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	if err := h.Agents.Disable(ctx, principal, input.AgentID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}

func (h *ManagementHandler) delete(ctx context.Context, input *agentPath) (*frameworkhttp.NoContent, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	if err := h.Agents.Delete(ctx, principal, input.AgentID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}

// Create 保留直接调用的 net/http 入口，与 Huma 共用保存逻辑。
func (h *ManagementHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.serveSave(w, r, "", http.StatusCreated)
}

// Save 保留直接调用的 net/http 入口，与 Huma 共用保存逻辑。
func (h *ManagementHandler) Save(w http.ResponseWriter, r *http.Request) {
	h.serveSave(w, r, r.PathValue("agent_id"), http.StatusOK)
}

func (h *ManagementHandler) serveSave(w http.ResponseWriter, r *http.Request, id string, status int) {
	if _, ok := requirePrincipal(w, r); !ok {
		return
	}
	var input agentRequest
	if !decodeAgentJSON(w, r, &input) {
		return
	}
	output, err := h.saveAgent(r.Context(), id, input)
	if err != nil {
		frameworkhttp.WriteAPIError(w, err)
		return
	}
	frameworkhttp.WriteJSON(w, status, output.Body)
}

func requirePrincipal(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	principal, ok := frameworkhttp.Principal(r.Context())
	if !ok || principal.UserID == "" {
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
		return auth.Principal{}, false
	}
	return principal, true
}

func decodeAgentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, agentBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		frameworkhttp.WriteError(w, r, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

func writeAgentError(w http.ResponseWriter, r *http.Request, err error) {
	frameworkhttp.WriteAPIError(w, agentError(r.Context(), err))
}

func agentError(ctx context.Context, err error) huma.StatusError {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		return frameworkhttp.APIError(ctx, http.StatusUnauthorized, "unauthenticated", err)
	case errors.Is(err, definition.ErrNotFound):
		return frameworkhttp.APIError(ctx, http.StatusNotFound, "agent_not_found", err)
	case errors.Is(err, definition.ErrForbidden):
		return frameworkhttp.APIError(ctx, http.StatusForbidden, "agent_forbidden", err)
	case errors.Is(err, definition.ErrConflict):
		return frameworkhttp.APIError(ctx, http.StatusConflict, "agent_conflict", err)
	case errors.Is(err, definition.ErrInvalid):
		return frameworkhttp.APIError(ctx, http.StatusUnprocessableEntity, "invalid_agent", err)
	default:
		return frameworkhttp.APIError(ctx, http.StatusServiceUnavailable, "agent_unavailable", err)
	}
}
