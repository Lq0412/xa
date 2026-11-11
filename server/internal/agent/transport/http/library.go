package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// LibraryHandler 只管理当前登录用户的公共 Agent 添加记录。
type LibraryHandler struct{ Library *definition.Library }

type libraryListResponse struct {
	Agents []definition.AgentDefinition `json:"agents" nullable:"false" doc:"当前用户库中可对话的 Agent"`
}

// RegisterRoutes 注册用户 Agent 库的读取和添加操作。
func (h *LibraryHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const path = "/api/v1/agent-library"
	operation := protectedAgentOperations(api, resolver, "Agent 库", "认证或 Agent 服务不可用（unauthenticated、agent_unavailable）")
	huma.Register(api, operation("listAgentLibrary", http.MethodGet, path, "获取用户 Agent 库", http.StatusOK, 0, 401, 503), h.list)
	huma.Register(api, operation("addAgentToLibrary", http.MethodPut, path+"/{agent_id}", "添加公开 Agent 到用户库", http.StatusNoContent, 0, 401, 404, 503), h.add)
}

func (h *LibraryHandler) list(ctx context.Context, _ *frameworkhttp.EmptyInput) (*routeOutput[libraryListResponse], error) {
	principal, _ := frameworkhttp.Principal(ctx)
	agents, err := h.Library.List(ctx, principal)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[libraryListResponse]{Body: libraryListResponse{Agents: agents}}, nil
}

func (h *LibraryHandler) add(ctx context.Context, input *agentPath) (*frameworkhttp.NoContent, error) {
	principal, _ := frameworkhttp.Principal(ctx)
	if err := h.Library.Add(ctx, principal, input.AgentID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}
