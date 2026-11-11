package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// MemoryHandler 所有操作从登录会话确定所有者。
type MemoryHandler struct {
	Memory *memory.Service
	Agents definition.Service
}

type memoryListInput struct {
	Scope     string          `query:"scope" enum:"user,agent,session,all_visible" doc:"记忆范围"`
	AgentID   string          `query:"agent_id" doc:"筛选指定 Agent"`
	Namespace string          `query:"namespace" doc:"筛选命名空间"`
	Status    string          `query:"status" enum:"active,deleted,superseded,archived" doc:"记忆状态，默认 active"`
	Limit     memoryListLimit `query:"limit" doc:"每页条数，默认 50，超过 200 按 200 处理"`
	Cursor    string          `query:"cursor" doc:"上页返回的游标"`
	paginated bool
}

type memoryListLimit string

// Schema 将 limit 文档化为整数，保留原始文本供处理器返回既有的错误码。
func (memoryListLimit) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeInteger}
}

// Resolve 使用原有 URL 查询解析规则，区分空的分页参数和没有分页参数。
func (input *memoryListInput) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	*input = readMemoryListInput(u.Query())
	return nil
}

func readMemoryListInput(q url.Values) memoryListInput {
	return memoryListInput{
		Scope: q.Get("scope"), AgentID: q.Get("agent_id"), Namespace: q.Get("namespace"), Status: q.Get("status"),
		Limit: memoryListLimit(q.Get("limit")), Cursor: q.Get("cursor"), paginated: q.Has("limit") || q.Has("cursor"),
	}
}

type memoryPath struct {
	MemoryID string `path:"memory_id" doc:"记忆条目 ID"`
}

type memoryJobPath struct {
	JobID string `path:"job_id" doc:"记忆任务 ID"`
}

type addMemoryRequest struct {
	Content    string `json:"content" doc:"记忆内容"`
	Namespace  string `json:"namespace" doc:"命名空间"`
	SubjectKey string `json:"subject_key,omitempty" doc:"主题键，省略时自动生成"`
	Kind       string `json:"kind,omitempty" enum:"fact,preference,episode,procedure" doc:"记忆类型，默认 fact"`
	Importance int    `json:"importance" doc:"重要性，取值 1 至 10"`
	Pinned     bool   `json:"pinned,omitempty" doc:"是否置顶"`
	Scope      string `json:"scope,omitempty" enum:"user,agent" doc:"记忆范围，默认 user"`
	AgentID    string `json:"agent_id,omitempty" doc:"Agent 范围对应的 ID"`
}

type addMemoryInput struct{ Body addMemoryRequest }

type editMemoryInput struct {
	MemoryID string `path:"memory_id" doc:"记忆条目 ID"`
	Body     memory.EditInput
}

type memoryEventsResponse struct {
	Events []memory.Event `json:"events" doc:"记忆取代链上的审计事件"`
}

type memoryPolicyRequest struct {
	ExpectedVersionID    string   `json:"expected_version_id" doc:"当前 Agent 配置版本 ID"`
	Enabled              *bool    `json:"enabled" nullable:"false" doc:"是否启用记忆"`
	CoreBudgetTokens     *int     `json:"core_budget_tokens,omitempty"`
	RecallTopK           *int     `json:"recall_top_k,omitempty"`
	MinRelevance         *float64 `json:"min_relevance,omitempty"`
	HistoryBudgetTokens  *int     `json:"history_budget_tokens,omitempty"`
	SummaryTriggerTokens *int     `json:"summary_trigger_tokens,omitempty"`
	SummaryBudgetTokens  *int     `json:"summary_budget_tokens,omitempty"`
	DecayEnabled         *bool    `json:"decay_enabled,omitempty"`
}

type memoryPolicyInput struct {
	AgentID string `path:"agent_id" doc:"Agent ID"`
	Body    memoryPolicyRequest
}

type memoryPolicyResponse struct {
	CurrentVersionID string                  `json:"current_version_id"`
	VersionNumber    int                     `json:"version_number"`
	MemoryPolicy     definition.MemoryPolicy `json:"memory_policy"`
}

// RegisterRoutes 注册记忆条目、策略和后台任务接口。
func (h *MemoryHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const memories = "/api/v1/memories"
	const item = memories + "/{memory_id}"
	const agent = "/api/v1/agents/{agent_id}"
	operation := protectedAgentOperations(api, resolver, "记忆", "认证或记忆服务不可用（unauthenticated、agent_unavailable）")
	huma.Register(api, operation("listMemories", http.MethodGet, memories, "列出当前用户的记忆", 200, 0, 401, 403, 404, 422, 503), h.list)
	huma.Register(api, operation("addMemory", http.MethodPost, memories, "新增手动记忆", 201, agentBodyLimit, 400, 401, 403, 404, 422, 503), h.add)
	huma.Register(api, operation("editMemory", http.MethodPut, item, "编辑记忆并保留原条目", 200, agentBodyLimit, 400, 401, 404, 409, 422, 503), h.edit)
	huma.Register(api, operation("deleteMemory", http.MethodDelete, item, "删除记忆", 204, 0, 401, 404, 503), h.delete)
	huma.Register(api, operation("restoreMemory", http.MethodPost, item+"/restore", "恢复记忆并保留原条目", 200, 0, 401, 404, 409, 422, 503), h.restore)
	huma.Register(api, operation("listMemoryEvents", http.MethodGet, item+"/events", "获取记忆取代链审计", 200, 0, 401, 404, 503), h.events)
	huma.Register(api, operation("getAgentMemory", http.MethodGet, agent+"/memory", "获取 Agent 的核心记忆", 200, 0, 401, 403, 404, 422, 503), h.core)
	huma.Register(api, operation("updateMemoryPolicy", http.MethodPut, agent+"/memory-policy", "更新 Agent 记忆策略", 200, agentBodyLimit, 400, 401, 403, 404, 409, 422, 503), h.policy)
	huma.Register(api, operation("getMemoryStats", http.MethodGet, "/api/v1/memory/stats", "统计当前用户的记忆和任务", 200, 0, 401, 503), h.stats)
	huma.Register(api, operation("retryMemoryJob", http.MethodPost, "/api/v1/memory/jobs/{job_id}/retry", "重新入队已终止的记忆任务", 204, 0, 401, 404, 503), h.retry)
}

func (h *MemoryHandler) list(ctx context.Context, input *memoryListInput) (*routeOutput[memory.Page], error) {
	p, _ := frameworkhttp.Principal(ctx)
	filter := memory.Filter{Scope: input.Scope, AgentID: input.AgentID, Namespace: input.Namespace, Status: input.Status}
	// 旧客户端不传分页参数，继续返回完整列表及原有优先级顺序。
	if !input.paginated && input.Scope != "all_visible" {
		items, err := h.Memory.List(ctx, p, filter)
		if err != nil {
			return nil, agentError(ctx, err)
		}
		return &routeOutput[memory.Page]{Body: memory.Page{Items: items}}, nil
	}
	limit := 50
	if input.Limit != "" {
		var err error
		if limit, err = strconv.Atoi(string(input.Limit)); err != nil {
			return nil, agentError(ctx, definition.ErrInvalid)
		}
	}
	page, err := h.Memory.ListPage(ctx, p, filter, input.Cursor, min(limit, 200))
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.Page]{Body: page}, nil
}

func (h *MemoryHandler) add(ctx context.Context, input *addMemoryInput) (*routeOutput[memory.Item], error) {
	p, _ := frameworkhttp.Principal(ctx)
	item, err := h.Memory.Add(ctx, p, memory.AddInput(input.Body))
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.Item]{Body: item}, nil
}

func (h *MemoryHandler) restore(ctx context.Context, input *memoryPath) (*routeOutput[memory.Item], error) {
	p, _ := frameworkhttp.Principal(ctx)
	item, err := h.Memory.Restore(ctx, p, input.MemoryID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.Item]{Body: item}, nil
}

func (h *MemoryHandler) stats(ctx context.Context, _ *frameworkhttp.EmptyInput) (*routeOutput[memory.Stats], error) {
	p, _ := frameworkhttp.Principal(ctx)
	stats, err := h.Memory.Stats(ctx, p)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.Stats]{Body: stats}, nil
}

func (h *MemoryHandler) retry(ctx context.Context, input *memoryJobPath) (*frameworkhttp.NoContent, error) {
	p, _ := frameworkhttp.Principal(ctx)
	if err := h.Memory.RetryJob(ctx, p, input.JobID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}

func (h *MemoryHandler) core(ctx context.Context, input *agentPath) (*routeOutput[memory.View], error) {
	p, _ := frameworkhttp.Principal(ctx)
	view, err := h.Memory.Core(ctx, p, input.AgentID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.View]{Body: view}, nil
}

func (h *MemoryHandler) edit(ctx context.Context, input *editMemoryInput) (*routeOutput[memory.Item], error) {
	p, _ := frameworkhttp.Principal(ctx)
	item, err := h.Memory.Edit(ctx, p, input.MemoryID, input.Body)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memory.Item]{Body: item}, nil
}

func (h *MemoryHandler) delete(ctx context.Context, input *memoryPath) (*frameworkhttp.NoContent, error) {
	p, _ := frameworkhttp.Principal(ctx)
	if err := h.Memory.Delete(ctx, p, input.MemoryID); err != nil {
		return nil, agentError(ctx, err)
	}
	return nil, nil
}

func (h *MemoryHandler) events(ctx context.Context, input *memoryPath) (*routeOutput[memoryEventsResponse], error) {
	p, _ := frameworkhttp.Principal(ctx)
	events, err := h.Memory.Events(ctx, p, input.MemoryID)
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memoryEventsResponse]{Body: memoryEventsResponse{Events: events}}, nil
}

func (h *MemoryHandler) policy(ctx context.Context, request *memoryPolicyInput) (*routeOutput[memoryPolicyResponse], error) {
	p, _ := frameworkhttp.Principal(ctx)
	input := request.Body
	if input.Enabled == nil {
		return nil, agentError(ctx, definition.ErrInvalid)
	}
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
	policy, err := definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
	if err != nil {
		return nil, agentError(ctx, err)
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
		return nil, agentError(ctx, err)
	}
	saved, err := h.Agents.Save(ctx, p, definition.SaveInput{AgentID: a.ID, ExpectedVersionID: input.ExpectedVersionID, Name: a.Name, Avatar: a.Avatar, Description: a.Description, Tags: a.Tags, Visibility: a.Visibility, Configuration: v.Configuration})
	if err != nil {
		return nil, agentError(ctx, err)
	}
	return &routeOutput[memoryPolicyResponse]{Body: memoryPolicyResponse{
		CurrentVersionID: saved.Version.ID, VersionNumber: saved.Version.VersionNumber, MemoryPolicy: policy,
	}}, nil
}

// List 保留直接调用的入口，使用相同的分页和过滤逻辑。
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	input := readMemoryListInput(r.URL.Query())
	serveLegacyJSON(w, r, http.StatusOK, &input, h.list)
}

// Add 保留直接调用的新增入口。
func (h *MemoryHandler) Add(w http.ResponseWriter, r *http.Request) {
	var input addMemoryInput
	if _, ok := requirePrincipal(w, r); !ok || !decodeAgentJSON(w, r, &input.Body) {
		return
	}
	out, err := h.add(r.Context(), &input)
	writeLegacyJSON(w, http.StatusCreated, out, err)
}

// Edit 保留直接调用的编辑入口。
func (h *MemoryHandler) Edit(w http.ResponseWriter, r *http.Request) {
	input := editMemoryInput{MemoryID: r.PathValue("memory_id")}
	if _, ok := requirePrincipal(w, r); !ok || !decodeAgentJSON(w, r, &input.Body) {
		return
	}
	out, err := h.edit(r.Context(), &input)
	writeLegacyJSON(w, http.StatusOK, out, err)
}

// Delete 保留直接调用的删除入口。
func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serveLegacyNoContent(w, r, &memoryPath{MemoryID: r.PathValue("memory_id")}, h.delete)
}

// Restore 保留直接调用的恢复入口。
func (h *MemoryHandler) Restore(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &memoryPath{MemoryID: r.PathValue("memory_id")}, h.restore)
}

// Events 保留直接调用的事件查询入口。
func (h *MemoryHandler) Events(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &memoryPath{MemoryID: r.PathValue("memory_id")}, h.events)
}

// Core 保留直接调用的核心记忆查询入口。
func (h *MemoryHandler) Core(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &agentPath{AgentID: r.PathValue("agent_id")}, h.core)
}

// Policy 保留直接调用的策略更新入口。
func (h *MemoryHandler) Policy(w http.ResponseWriter, r *http.Request) {
	input := memoryPolicyInput{AgentID: r.PathValue("agent_id")}
	if _, ok := requirePrincipal(w, r); !ok || !decodeAgentJSON(w, r, &input.Body) {
		return
	}
	out, err := h.policy(r.Context(), &input)
	writeLegacyJSON(w, http.StatusOK, out, err)
}

// Stats 保留直接调用的统计入口。
func (h *MemoryHandler) Stats(w http.ResponseWriter, r *http.Request) {
	serveLegacyJSON(w, r, http.StatusOK, &frameworkhttp.EmptyInput{}, h.stats)
}

// Retry 保留直接调用的任务重试入口。
func (h *MemoryHandler) Retry(w http.ResponseWriter, r *http.Request) {
	serveLegacyNoContent(w, r, &memoryJobPath{JobID: r.PathValue("job_id")}, h.retry)
}
