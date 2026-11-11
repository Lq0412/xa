package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
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

type toolCatalogApproval struct {
	Default     tool.ApprovalMode `json:"default" enum:"never,always"`
	Overridable bool              `json:"overridable"`
}

type toolCatalogItem struct {
	Name           string              `json:"name"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	Effect         tool.Effect         `json:"effect" enum:"read,write,destructive"`
	DefaultEnabled bool                `json:"default_enabled"`
	Approval       toolCatalogApproval `json:"approval"`
	Available      bool                `json:"available"`
	ConfigSchema   any                 `json:"config_schema" doc:"工具特定的配置 JSON Schema"`
	InputSchema    any                 `json:"input_schema" doc:"工具特定的输入 JSON Schema"`
}

type toolCatalogResponse struct {
	Tools []toolCatalogItem `json:"tools" nullable:"false"`
}

type runtimeLimitsRequest json.RawMessage

// Schema 说明客户端可只提供部分预算，默认值和上限由服务解析。
func (runtimeLimitsRequest) Schema(registry huma.Registry) *huma.Schema {
	schema := *registry.Schema(reflect.TypeFor[definition.RuntimeLimits](), false, "RuntimeLimits")
	schema.Required = nil
	schema.Nullable = true
	schema.AdditionalProperties = true
	return &schema
}

func (r *runtimeLimitsRequest) UnmarshalJSON(data []byte) error {
	return (*json.RawMessage)(r).UnmarshalJSON(data)
}

type updateAgentToolsRequest struct {
	ExpectedVersionID string                `json:"expected_version_id" doc:"当前 Agent 配置版本 ID"`
	Tools             []binding.ToolBinding `json:"tools" doc:"Agent 的工具绑定"`
	RuntimeLimits     runtimeLimitsRequest  `json:"runtime_limits,omitempty" doc:"运行预算，省略时保留当前预算"`
}

type updateAgentToolsInput struct {
	AgentID string `path:"agent_id" doc:"Agent ID"`
	Body    updateAgentToolsRequest
}

type agentToolEntry struct {
	Name                string            `json:"name"`
	Title               string            `json:"title"`
	Enabled             bool              `json:"enabled"`
	Approval            tool.ApprovalMode `json:"approval" enum:"never,always"`
	TimeoutMS           int64             `json:"timeout_ms"`
	MaxCalls            int               `json:"max_calls"`
	Configuration       any               `json:"configuration" doc:"工具特定的生效配置"`
	Available           bool              `json:"available"`
	Reason              string            `json:"reason"`
	ApprovalOverridable bool              `json:"approval_overridable"`
}

type agentToolsResponse struct {
	CurrentVersionID string                   `json:"current_version_id"`
	VersionNumber    *int                     `json:"version_number,omitempty" doc:"更新后返回的新版本序号"`
	Tools            []agentToolEntry         `json:"tools" nullable:"false"`
	RuntimeLimits    definition.RuntimeLimits `json:"runtime_limits"`
	ToolsetHash      string                   `json:"toolset_hash"`
}

// RegisterRoutes 注册工具目录和 Agent 工具配置接口。
func (h *ToolsHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const path = "/api/v1/agents/{agent_id}/tools"
	const tag = "工具"
	operation := protectedAgentOperations(api, resolver, tag, "认证或 Agent 服务不可用（unauthenticated、agent_unavailable）")
	catalog := protectedAgentOperations(api, resolver, tag, "认证或工具目录不可用（unauthenticated、tool_unavailable）")
	huma.Register(api, catalog("listTools", http.MethodGet, "/api/v1/tools", "获取平台工具目录", http.StatusOK, 0, 401, 503), h.catalog)
	huma.Register(api, operation("getAgentTools", http.MethodGet, path, "获取 Agent 当前工具配置", http.StatusOK, 0, 401, 403, 404, 422, 503), h.getAgentTools)
	huma.Register(api, operation("updateAgentTools", http.MethodPut, path, "更新 Agent 工具配置", http.StatusOK, agentBodyLimit, 400, 401, 403, 404, 409, 422, 503), h.putAgentTools)
}

func (h *ToolsHandler) catalog(ctx context.Context, _ *frameworkhttp.EmptyInput) (*routeOutput[toolCatalogResponse], error) {
	if h.Registry == nil {
		return nil, frameworkhttp.APIError(ctx, http.StatusServiceUnavailable, "tool_unavailable", errors.New("工具目录未初始化"))
	}
	specs := h.Registry.Catalog()
	items := make([]toolCatalogItem, 0, len(specs))
	for _, spec := range specs {
		items = append(items, toolCatalogItem{
			Name: spec.Name, Title: spec.Title, Description: spec.Description, Effect: spec.Effect,
			DefaultEnabled: spec.DefaultEnabled, Approval: toolCatalogApproval{Default: spec.Approval.Default, Overridable: spec.Approval.Overridable},
			Available: spec.Available, ConfigSchema: jsonOrEmpty(spec.ConfigSchema), InputSchema: jsonOrEmpty(spec.InputSchema),
		})
	}
	return &routeOutput[toolCatalogResponse]{Body: toolCatalogResponse{Tools: items}}, nil
}

func (h *ToolsHandler) getAgentTools(ctx context.Context, input *agentPath) (*routeOutput[agentToolsResponse], error) {
	p, _ := frameworkhttp.Principal(ctx)
	view, err := h.agentToolsView(ctx, p, input.AgentID, true)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[agentToolsResponse]{Body: view}, nil
}

func (h *ToolsHandler) putAgentTools(ctx context.Context, request *updateAgentToolsInput) (*routeOutput[agentToolsResponse], error) {
	p, _ := frameworkhttp.Principal(ctx)
	input := request.Body
	a, err := h.Agents.Get(ctx, p, request.AgentID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	if a.OwnerUserID != p.UserID {
		return nil, agentError(ctx, definition.ErrForbidden)
	}
	v, err := h.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	v.Configuration.Tools = input.Tools
	if len(input.RuntimeLimits) > 0 {
		limits, err := definition.ParseRuntimeLimits(json.RawMessage(input.RuntimeLimits))
		if err != nil {
			return nil, agentError(ctx, err)
		}
		v.Configuration.RuntimeLimits, err = json.Marshal(limits)
		if err != nil {
			return nil, agentError(ctx, definition.ErrInvalid)
		}
	}
	saved, err := h.Agents.Save(ctx, p, definition.SaveInput{
		AgentID: a.ID, ExpectedVersionID: input.ExpectedVersionID,
		Name: a.Name, Avatar: a.Avatar, Description: a.Description, Tags: a.Tags, Visibility: a.Visibility,
		Configuration: v.Configuration,
	})
	if err != nil {
		return nil, agentError(ctx, err)
	}
	view, err := h.agentToolsView(ctx, p, a.ID, false)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	view.CurrentVersionID = saved.Version.ID
	view.VersionNumber = &saved.Version.VersionNumber
	return &routeOutput[agentToolsResponse]{Body: view}, nil
}

func (h *ToolsHandler) agentToolsView(ctx context.Context, p auth.Principal, agentID string, hideOtherOwners bool) (agentToolsResponse, error) {
	a, err := h.Agents.Get(ctx, p, agentID)
	if err != nil {
		return agentToolsResponse{}, err
	}
	if hideOtherOwners && a.OwnerUserID != p.UserID {
		return agentToolsResponse{}, definition.ErrForbidden
	}
	v, err := h.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return agentToolsResponse{}, err
	}
	policy, err := definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
	if err != nil {
		return agentToolsResponse{}, err
	}
	limits, err := definition.ParseRuntimeLimits(v.Configuration.RuntimeLimits)
	if err != nil {
		return agentToolsResponse{}, err
	}
	toolset, err := h.Registry.Resolve(tool.ResolveInput{
		Bindings: v.Configuration.Tools, MemoryPolicy: policy, Principal: p, CanApprove: true,
	})
	if err != nil {
		return agentToolsResponse{}, err
	}
	tools := make([]agentToolEntry, 0, len(toolset.Entries))
	for _, e := range toolset.Entries {
		tools = append(tools, agentToolEntry{
			Name: e.Spec.Name, Title: e.Spec.Title, Enabled: e.Enabled, Approval: e.Approval,
			TimeoutMS: e.Timeout.Milliseconds(), MaxCalls: e.MaxCalls, Configuration: jsonOrEmpty(e.Config),
			Available: e.Spec.Available, Reason: e.Reason, ApprovalOverridable: e.Spec.Approval.Overridable,
		})
	}
	return agentToolsResponse{CurrentVersionID: a.CurrentVersionID, Tools: tools, RuntimeLimits: limits, ToolsetHash: toolset.Hash}, nil
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

// Catalog 保留直接调用的目录入口。
func (h *ToolsHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &frameworkhttp.EmptyInput{}, h.catalog)
}

// GetAgentTools 保留直接调用的工具配置查询入口。
func (h *ToolsHandler) GetAgentTools(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &agentPath{AgentID: r.PathValue("agent_id")}, h.getAgentTools)
}

// PutAgentTools 保留直接调用的工具配置更新入口。
func (h *ToolsHandler) PutAgentTools(w http.ResponseWriter, r *http.Request) {
	input := updateAgentToolsInput{AgentID: r.PathValue("agent_id")}
	if _, ok := requirePrincipal(w, r); !ok || !decodeAgentJSON(w, r, &input.Body) {
		return
	}
	out, err := h.putAgentTools(r.Context(), &input)
	writeLegacyJSON(w, http.StatusOK, out, err)
}
