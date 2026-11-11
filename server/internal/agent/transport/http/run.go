package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// RunHandler 处理 run 事件流与取消。
type RunHandler struct {
	Conversations *conversation.Service
	Runs          *run.RunService
}

const runPath = "/api/v1/runs"

type runIDInput struct {
	RunID string `path:"run_id" doc:"运行 ID"`
}

type cancelResult struct {
	Status string `json:"status"`
}

type cancelOutput struct {
	Status int
	Body   cancelResult
}

type decisionBody struct {
	Decision string `json:"decision" enum:"approve,reject"`
	Reason   string `json:"reason,omitempty" maxLength:"200"`
	Remember string `json:"remember,omitempty" enum:"once,conversation,agent"`
}

type decisionInput struct {
	ApprovalID string `path:"approval_id" doc:"审批 ID"`
	Body       decisionBody
}

// RegisterRoutes 注册运行、审批和 Agent 活动的 JSON 接口。
func (h *RunHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	runOperation := agentProtectedOperation(api, resolver, "运行")
	approvalOperation := agentProtectedOperation(api, resolver, "审批")
	cancel := runOperation("cancelRun", http.MethodPost, runPath+"/{run_id}/cancel", "取消运行", 200, 0, map[int]string{
		404: "运行不存在（not_found）", 409: "审批已决定（run_not_waiting）", 500: "运行服务不可用（run_unavailable、conversation_unavailable）",
	})
	cancel.Responses["202"] = &huma.Response{Description: "取消请求已接受，运行正在停止", Content: map[string]*huma.MediaType{
		"application/json": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[cancelResult](), true, "CancelResult")},
	}}
	huma.Register(api, cancel, h.cancel)
	huma.Register(api, approvalOperation("decideApproval", http.MethodPost, "/api/v1/approvals/{approval_id}/decision", "允许或拒绝工具执行", 200, agentBodyLimit, map[int]string{
		400: "请求 JSON 或审批决定无效（invalid_json、invalid_decision）", 404: "审批不存在（not_found）",
		409: "审批状态冲突（approval_conflict、run_not_waiting、conflict）", 500: "审批服务不可用（conversation_unavailable）",
	}), h.decision)
	huma.Register(api, approvalOperation("listAdminApprovals", http.MethodGet, "/api/v1/admin/approvals", "列出工作空间审批记录", 200, 0, map[int]string{
		400: "筛选或分页参数无效（invalid_status、invalid_limit）", 403: "仅管理员可查看（forbidden）", 500: "审批记录不可用（approvals_unavailable）",
	}), h.approvalRecords)
	huma.Register(api, approvalOperation("listUserApprovals", http.MethodGet, "/api/v1/approvals", "列出我的审批", 200, 0, map[int]string{
		400: "分页参数无效（invalid_limit、invalid_cursor）", 500: "审批记录不可用（approvals_unavailable、internal_error）",
	}), h.userApprovals)
	huma.Register(api, runOperation("getAgentActivity", http.MethodGet, "/api/v1/agents/{agent_id}/activity", "列出 Agent 活动", 200, 0, map[int]string{
		400: "分页参数无效（invalid_limit、invalid_cursor）", 404: "Agent 不存在（not_found）", 500: "活动记录不可用（activity_unavailable、internal_error）",
	}), h.agentActivity)
	h.registerRunQueries(api, runOperation)
}

// RegisterStreams 声明运行事件流，由 net/http 处理心跳和重连。
func (h *RunHandler) RegisterStreams(api huma.API) {
	registry := api.OpenAPI().Components.Schemas
	op := frameworkhttp.Operation(api, "运行", "subscribeRunEvents", http.MethodGet, runPath+"/{run_id}/events", "订阅运行事件", 200, 0, map[int]string{
		400: "事件序号无效（invalid_after）", 401: "会话无效（unauthenticated）", 404: "运行不存在（not_found）",
		500: "运行服务或流式响应不可用（run_unavailable、conversation_unavailable、stream_unsupported）",
		503: "认证服务不可用（unauthenticated）",
	})
	op.Security = []map[string][]string{{"bearerAuth": {}}}
	op.Description = "使用 v2 事件信封。读取 after 与有效 Last-Event-ID 两者中较大的序号之后的事件，连接可收到 SSE 注释心跳。"
	op.Parameters = []*huma.Param{
		streamParam("run_id", "path", "运行 ID", huma.TypeString),
		streamParam("after", "query", "从此非负序号之后开始读取，默认 0", huma.TypeInteger),
		streamParam("Last-Event-ID", "header", "客户端已收到的最后事件序号", huma.TypeInteger),
	}
	op.Responses["200"] = streamResponse("SSE 事件流，Schema 描述每条事件的 data 字段",
		streamEnvelopeSchema(registry), registry.Schema(reflect.TypeFor[streamError](), true, "StreamError"),
	)
	api.OpenAPI().AddOperation(&op)
}

// Events 向运行所有者重放并转发 v2 事件。
func (h *RunHandler) Events(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	runID := r.PathValue("run_id")
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			frameworkhttp.WriteError(w, r, http.StatusBadRequest, "invalid_after", fmt.Errorf("after 无效"))
			return
		}
		after = v
	}
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > after {
			after = v
		}
	}
	// 先校验所有者，再开始 SSE；没有新事件的审批连接也应立即确认已连接。
	if h.Conversations == nil || h.Conversations.Runs == nil {
		frameworkhttp.WriteError(w, r, 500, "run_unavailable", fmt.Errorf("RunService 未配置"))
		return
	}
	if _, err := h.Conversations.Runs.QueryRun(r.Context(), p, runID, false); err != nil {
		conversationError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		frameworkhttp.WriteError(w, r, 500, "stream_unsupported", fmt.Errorf("不支持流式响应"))
		return
	}
	if err := run.WriteSSEPing(w, flusher); err != nil {
		return
	}
	err := h.Conversations.SubscribeRun(r.Context(), p, runID, conversation.StreamSendOptions{
		Protocol: 2, AfterSeq: after,
		WriteV2: func(ev run.StoredEvent) error {
			return run.WriteSSEEnvelope(w, flusher, ev)
		},
	})
	if err != nil {
		b, _ := json.Marshal(streamError{Message: conversation.PublicError(err)})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
		flusher.Flush()
	}
}

// Cancel 为直接调用的 net/http 入口复用运行取消逻辑。
func (h *RunHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	output, err := h.cancel(r.Context(), &runIDInput{RunID: r.PathValue("run_id")})
	if err != nil {
		frameworkhttp.WriteAPIError(w, err.(huma.StatusError))
		return
	}
	frameworkhttp.WriteJSON(w, output.Status, output.Body)
}

func (h *RunHandler) cancel(ctx context.Context, input *runIDInput) (*cancelOutput, error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if h.Runs == nil {
		return nil, frameworkhttp.APIError(ctx, 500, "run_unavailable", fmt.Errorf("RunService 未配置"))
	}
	status, err := h.Runs.Cancel(ctx, p, input.RunID)
	if err != nil {
		return nil, runActionError(ctx, err)
	}
	code := 200
	if status == "cancelling" {
		code = 202
	}
	return &cancelOutput{Status: code, Body: cancelResult{Status: status}}, nil
}

// Decision 只接受 owner 本人的审批决定。
func (h *RunHandler) Decision(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePrincipal(w, r); !ok {
		return
	}
	in := decisionInput{ApprovalID: r.PathValue("approval_id")}
	if !decodeAgentJSON(w, r, &in.Body) {
		return
	}
	output, err := h.decision(r.Context(), &in)
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) decision(ctx context.Context, input *decisionInput) (*routeOutput[run.DecisionResult], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Runs.Decide(ctx, p, input.ApprovalID, run.DecisionInput(input.Body))
	if err != nil {
		return nil, runActionError(ctx, err)
	}
	return &routeOutput[run.DecisionResult]{Body: result}, nil
}

func runActionError(ctx context.Context, err error) huma.StatusError {
	var approvalErr *run.ApprovalError
	if errors.As(err, &approvalErr) {
		return frameworkhttp.APIError(ctx, approvalErr.Status, approvalErr.Code, err)
	}
	return mapConversationError(ctx, err)
}
