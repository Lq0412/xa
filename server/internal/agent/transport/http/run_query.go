package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

type runListInput struct {
	ConversationID string `query:"conversation_id" doc:"按会话 ID 筛选"`
	AgentID        string `query:"agent_id" doc:"按 Agent ID 筛选"`
	Status         string `query:"status" doc:"按运行状态筛选" enum:"queued,running,waiting_approval,completed,failed,cancelled,budget_exhausted,interrupted"`
	UserID         string `query:"user_id" doc:"按用户 ID 筛选，仅管理员可用"`
	Cursor         string `query:"cursor" doc:"上一页返回的 next_cursor"`
	Limit          string `query:"limit" doc:"每页条数，范围 1 至 100，默认 20"`
}

type toolResultInput struct {
	RunID  string `path:"run_id" doc:"运行 ID"`
	CallID string `path:"call_id" doc:"工具调用 ID"`
}

type spanTreeResponse run.SpanTree

// Schema 保留历史运行返回的 span:null。Huma 默认不把结构体指针声明为可空。
func (spanTreeResponse) Schema(registry huma.Registry) *huma.Schema {
	schema := huma.SchemaFromType(registry, reflect.TypeFor[run.SpanTree]())
	schema.Properties["span"] = &huma.Schema{OneOf: []*huma.Schema{
		schema.Properties["span"], {Type: huma.TypeNull},
	}}
	return schema
}

func (h *RunHandler) registerRunQueries(api huma.API, operation protectedOperation) {
	for _, route := range []struct {
		base, listID, getID, spansID, listSummary string
		admin                                     bool
	}{
		{runPath, "listRuns", "getRun", "getRunSpans", "列出我的运行", false},
		{"/api/v1/admin/runs", "listAdminRuns", "getAdminRun", "getAdminRunSpans", "列出工作空间运行", true},
	} {
		listErrors := map[int]string{400: "筛选或分页参数无效（invalid_request、invalid_limit）", 500: "运行查询不可用（conversation_unavailable）"}
		getErrors := map[int]string{404: "运行不存在（not_found）", 500: "运行查询不可用（conversation_unavailable）"}
		spanErrors := map[int]string{404: "运行不存在（not_found）", 500: "执行树查询不可用（conversation_unavailable）"}
		if route.admin {
			for _, descriptions := range []map[int]string{listErrors, getErrors, spanErrors} {
				descriptions[403] = "仅管理员可查看（forbidden）"
			}
		}
		huma.Register(api, operation(route.listID, http.MethodGet, route.base, route.listSummary, 200, 0, listErrors),
			func(ctx context.Context, in *runListInput) (*routeOutput[run.RunList], error) {
				return h.listRuns(ctx, in, route.admin)
			})
		huma.Register(api, operation(route.getID, http.MethodGet, route.base+"/{run_id}", "获取运行详情", 200, 0, getErrors),
			func(ctx context.Context, in *runIDInput) (*routeOutput[run.RunDetail], error) {
				return h.getRun(ctx, in, route.admin)
			})
		huma.Register(api, operation(route.spansID, http.MethodGet, route.base+"/{run_id}/spans", "获取运行执行树", 200, 0, spanErrors),
			func(ctx context.Context, in *runIDInput) (*routeOutput[spanTreeResponse], error) {
				return h.spans(ctx, in, route.admin)
			})
	}
	huma.Register(api, operation("getToolResult", http.MethodGet, runPath+"/{run_id}/tool-results/{call_id}", "获取工具调用结果", 200, 0, map[int]string{
		404: "运行或工具调用不存在（not_found）", 500: "工具结果查询不可用（conversation_unavailable）",
	}), h.toolResult)
}

// ListRuns 共享 owner 与管理路径，管理路径的角色校验由查询服务执行。
func (h *RunHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	output, err := h.listRuns(r.Context(), &runListInput{
		ConversationID: query.Get("conversation_id"), AgentID: query.Get("agent_id"), Status: query.Get("status"),
		UserID: query.Get("user_id"), Cursor: query.Get("cursor"), Limit: query.Get("limit"),
	}, isAdminRunPath(r))
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) listRuns(ctx context.Context, input *runListInput, admin bool) (*routeOutput[run.RunList], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	limit := 20
	if input.Limit != "" {
		n, err := strconv.Atoi(input.Limit)
		if err != nil || n < 1 || n > 100 {
			return nil, frameworkhttp.APIError(ctx, 400, "invalid_limit", fmt.Errorf("limit 无效"))
		}
		limit = n
	}
	page, err := h.Runs.ListRuns(ctx, p, run.RunFilter{
		ConversationID: input.ConversationID, AgentID: input.AgentID, Status: input.Status, UserID: input.UserID, Cursor: input.Cursor, Limit: limit,
	}, admin)
	if errors.Is(err, definition.ErrInvalid) {
		return nil, frameworkhttp.APIError(ctx, 400, "invalid_request", fmt.Errorf("运行筛选参数无效"))
	}
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[run.RunList]{Body: page}, nil
}

// GetRun 返回本次运行摘要和允许出站的配置。
func (h *RunHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	output, err := h.getRun(r.Context(), &runIDInput{RunID: r.PathValue("run_id")}, isAdminRunPath(r))
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) getRun(ctx context.Context, input *runIDInput, admin bool) (*routeOutput[run.RunDetail], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	value, err := h.Runs.QueryRun(ctx, p, input.RunID, admin)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[run.RunDetail]{Body: value}, nil
}

// Spans 返回脱敏执行树；历史未追踪运行的 span 为 null。
func (h *RunHandler) Spans(w http.ResponseWriter, r *http.Request) {
	output, err := h.spans(r.Context(), &runIDInput{RunID: r.PathValue("run_id")}, isAdminRunPath(r))
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) spans(ctx context.Context, input *runIDInput, admin bool) (*routeOutput[spanTreeResponse], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	tree, err := h.Runs.QuerySpans(ctx, p, input.RunID, admin)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[spanTreeResponse]{Body: spanTreeResponse(tree)}, nil
}

// ToolResult 只注册 owner 路径，完整载荷不提供管理员入口。
func (h *RunHandler) ToolResult(w http.ResponseWriter, r *http.Request) {
	output, err := h.toolResult(r.Context(), &toolResultInput{RunID: r.PathValue("run_id"), CallID: r.PathValue("call_id")})
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) toolResult(ctx context.Context, input *toolResultInput) (*routeOutput[run.ToolResultDetail], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	value, err := h.Runs.QueryToolResult(ctx, p, input.RunID, input.CallID)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[run.ToolResultDetail]{Body: value}, nil
}

func isAdminRunPath(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/api/v1/admin/runs") }
