package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// ConversationHandler 提供会话查询和消息流。
type ConversationHandler struct{ Conversations *conversation.Service }

const conversationPath = "/api/v1/conversations"

type createConversationBody struct {
	AgentID string `json:"agent_id" doc:"Agent ID"`
}

type createConversationInput struct {
	RawBody []byte `contentType:"application/json"`
}

type conversationList struct {
	Conversations []conversation.Conversation `json:"conversations" nullable:"false"`
}

type conversationIDInput struct {
	ConversationID string `path:"conversation_id" doc:"会话 ID"`
}

type messagePageInput struct {
	ConversationID string `path:"conversation_id" doc:"会话 ID"`
	Before         string `query:"before" doc:"从此消息之前开始读取"`
	Limit          string `query:"limit" doc:"每页条数，范围 1 至 100，默认 50"`
}

type conversationSummary struct {
	value *conversation.Summary
}

type missingConversationSummary struct {
	Summary *struct{} `json:"summary"`
}

// Schema 对应既有摘要协议：完整摘要对象，或只有 summary:null 的对象。
func (conversationSummary) Schema(registry huma.Registry) *huma.Schema {
	missing := registry.Schema(reflect.TypeFor[missingConversationSummary](), false, "MissingConversationSummary")
	missing.Properties["summary"] = &huma.Schema{Type: huma.TypeNull}
	return &huma.Schema{OneOf: []*huma.Schema{
		registry.Schema(reflect.TypeFor[conversation.Summary](), true, "Summary"),
		missing,
	}}
}

func (s conversationSummary) MarshalJSON() ([]byte, error) {
	if s.value == nil {
		return json.Marshal(missingConversationSummary{})
	}
	return json.Marshal(s.value)
}

// RegisterRoutes 注册会话的 JSON 接口。
func (h *ConversationHandler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const tag = "会话"
	const entry = conversationPath + "/{conversation_id}"
	operation := agentProtectedOperation(api, resolver, tag)
	create := operation("createConversation", http.MethodPost, conversationPath, "创建或获取会话", 201, -1, map[int]string{
		400: "请求 JSON 无效（invalid_json）", 403: "没有 Agent 访问权限（forbidden）",
		404: "Agent 不存在（not_found）", 500: "会话服务不可用（conversation_unavailable）",
	})
	create.RequestBody = &huma.RequestBody{}
	huma.Register(api, create, h.create)
	// RawBody 交给旧解码规则处理，文档仍使用实际请求字段生成。
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[createConversationBody](), false, "CreateConversationBody")
	schema.AdditionalProperties = true
	create.RequestBody.Content["application/json"].Schema = schema
	huma.Register(api, operation("listConversations", http.MethodGet, conversationPath, "列出我的会话", 200, 0, map[int]string{
		500: "会话服务不可用（conversation_unavailable）",
	}), h.list)
	huma.Register(api, operation("getConversationSummary", http.MethodGet, entry+"/summary", "获取会话摘要", 200, 0, map[int]string{
		404: "会话不存在（not_found）", 500: "会话服务不可用（conversation_unavailable）",
	}), h.summary)
	huma.Register(api, operation("listConversationMessages", http.MethodGet, entry+"/messages", "列出会话消息", 200, 0, map[int]string{
		400: "分页条数无效（invalid_limit）", 404: "会话不存在（not_found）",
		422: "消息游标无效（invalid_request）", 500: "会话服务不可用（conversation_unavailable）",
	}), h.messages)
}

type protectedOperation func(id, method, path, summary string, status int, maxBodyBytes int64, errors map[int]string) huma.Operation

// agentProtectedOperation 将会话和运行接口的认证声明与现有认证中间件绑定。
func agentProtectedOperation(api huma.API, resolver auth.Resolver, tag string) protectedOperation {
	protect := func(next http.Handler) http.Handler { return frameworkhttp.Authenticate(resolver, next) }
	return func(id, method, path, summary string, status int, maxBodyBytes int64, errors map[int]string) huma.Operation {
		errors[401] = "会话无效（unauthenticated）"
		errors[503] = "认证服务不可用（unauthenticated）"
		return frameworkhttp.OperationAuth(frameworkhttp.Operation(api, tag, id, method, path, summary, status, maxBodyBytes, errors), protect, false)
	}
}

func (h *ConversationHandler) create(ctx context.Context, input *createConversationInput) (*routeOutput[conversation.Conversation], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var in createConversationBody
	if err := json.NewDecoder(bytes.NewReader(input.RawBody)).Decode(&in); err != nil {
		return nil, frameworkhttp.APIError(ctx, 400, "invalid_json", err)
	}
	c, err := h.Conversations.Create(ctx, p, in.AgentID)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[conversation.Conversation]{Body: c}, nil
}

func (h *ConversationHandler) list(ctx context.Context, _ *frameworkhttp.EmptyInput) (*routeOutput[conversationList], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.Conversations.List(ctx, p)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[conversationList]{Body: conversationList{Conversations: v}}, nil
}

// Summary 为直接调用的 net/http 入口复用会话摘要查询。
func (h *ConversationHandler) Summary(w http.ResponseWriter, r *http.Request) {
	output, err := h.summary(r.Context(), &conversationIDInput{ConversationID: r.PathValue("conversation_id")})
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *ConversationHandler) summary(ctx context.Context, input *conversationIDInput) (*routeOutput[conversationSummary], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.Conversations.Summary(ctx, p, input.ConversationID)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[conversationSummary]{Body: conversationSummary{value: v}}, nil
}

func (h *ConversationHandler) messages(ctx context.Context, input *messagePageInput) (*routeOutput[conversation.MessagePage], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if input.Limit != "" {
		parsed, err := strconv.Atoi(input.Limit)
		if err != nil || parsed < 1 || parsed > 100 {
			return nil, frameworkhttp.APIError(ctx, 400, "invalid_limit", definition.ErrInvalid)
		}
		limit = parsed
	}
	v, err := h.Conversations.Messages(ctx, p, input.ConversationID, input.Before, limit)
	if err != nil {
		return nil, mapConversationError(ctx, err)
	}
	return &routeOutput[conversation.MessagePage]{Body: v}, nil
}

func routePrincipal(ctx context.Context) (auth.Principal, error) {
	p, ok := frameworkhttp.Principal(ctx)
	if !ok || p.UserID == "" {
		return auth.Principal{}, frameworkhttp.APIError(ctx, 401, "unauthenticated", auth.ErrUnauthenticated)
	}
	return p, nil
}

type sendMessageBody struct {
	Content string `json:"content" doc:"用户消息正文"`
}

type streamDelta struct {
	Delta string `json:"delta"`
}

type streamDone struct {
	CompletionTokens int    `json:"completion_tokens"`
	MessageID        string `json:"message_id"`
	PromptTokens     int    `json:"prompt_tokens"`
}

type streamError struct {
	Message string `json:"message"`
}

// RegisterStreams 声明消息流协议，由 net/http 入口负责实际写出事件。
func (h *ConversationHandler) RegisterStreams(api huma.API) {
	registry := api.OpenAPI().Components.Schemas
	op := frameworkhttp.Operation(api, "会话", "sendConversationMessage", http.MethodPost, conversationPath+"/{conversation_id}/messages", "发送消息并订阅回复", 200, 0, map[int]string{
		400: "请求 JSON 无效（invalid_json）", 401: "会话无效（unauthenticated）",
		404: "会话不存在（not_found）", 409: "会话已有运行或正在等待审批（conflict、run_waiting_approval）",
		422: "消息过长或幂等键无效（message_too_long、invalid_request、idempotency_key_reused）",
		500: "会话服务或流式响应不可用（conversation_unavailable、stream_unsupported）",
		503: "认证服务不可用（unauthenticated）",
	})
	op.Security = []map[string][]string{{"bearerAuth": {}}}
	op.Description = "默认使用 v1 的 delta、done 和 error 事件。通过 X-XA-Stream 或 stream 选择 v2 后，事件使用运行信封，可用 Last-Event-ID 从指定序号之后继续读取。"
	op.Parameters = []*huma.Param{
		streamParam("conversation_id", "path", "会话 ID", huma.TypeString),
		streamParam("stream", "query", "传 2 选择 v2 事件协议", huma.TypeString),
		streamParam("X-XA-Stream", "header", "传 2 选择 v2 事件协议", huma.TypeString),
		streamParam("Idempotency-Key", "header", "v2 发送请求的幂等键", huma.TypeString),
		streamParam("Last-Event-ID", "header", "v2 已收到的最后事件序号", huma.TypeInteger),
	}
	op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
		"application/json": {Schema: registry.Schema(reflect.TypeFor[sendMessageBody](), true, "SendMessageBody")},
	}}
	conflict := op.Responses["409"].Content["application/json"]
	conflict.Schema = &huma.Schema{OneOf: []*huma.Schema{
		conflict.Schema, registry.Schema(reflect.TypeFor[waitingApprovalResponse](), true, "WaitingApprovalResponse"),
	}}
	op.Responses["200"] = streamResponse("SSE 事件流，Schema 描述每条事件的 data 字段",
		registry.Schema(reflect.TypeFor[streamDelta](), true, "StreamDelta"),
		registry.Schema(reflect.TypeFor[streamDone](), true, "StreamDone"),
		registry.Schema(reflect.TypeFor[streamError](), true, "StreamError"),
		streamEnvelopeSchema(registry),
	)
	api.OpenAPI().AddOperation(&op)
}

func streamParam(name, location, description, schemaType string) *huma.Param {
	return &huma.Param{Name: name, In: location, Description: description, Required: location == "path", Schema: &huma.Schema{Type: schemaType}}
}

func streamResponse(description string, variants ...*huma.Schema) *huma.Response {
	return &huma.Response{Description: description, Content: map[string]*huma.MediaType{
		"text/event-stream": {Schema: &huma.Schema{OneOf: variants}},
	}}
}

func streamEnvelopeSchema(registry huma.Registry) *huma.Schema {
	// v2 信封会平铺各事件自己的字段。
	schema := registry.Schema(reflect.TypeFor[run.Envelope](), false, "Envelope")
	schema.AdditionalProperties = true
	return schema
}

// Send 保留两版 SSE 写出协议。
func (h *ConversationHandler) Send(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	var in sendMessageBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		frameworkhttp.WriteError(w, r, 400, "invalid_json", err)
		return
	}
	protocol := 1
	if r.Header.Get("X-XA-Stream") == "2" || r.URL.Query().Get("stream") == "2" {
		protocol = 2
	}
	if protocol >= 2 {
		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if err := h.Conversations.CheckIdempotencyKey(r.Context(), r.PathValue("conversation_id"), in.Content, idemKey, 2); err != nil {
			conversationError(w, r, err)
			return
		}
		if idemKey == "" {
			if err := h.Conversations.EnsureNoActiveRun(r.Context(), p, r.PathValue("conversation_id")); err != nil {
				conversationError(w, r, err)
				return
			}
		}
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
	if protocol >= 2 {
		opts := conversation.StreamSendOptions{
			Protocol:       2,
			IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			WriteV2: func(ev run.StoredEvent) error {
				return run.WriteSSEEnvelope(w, flusher, ev)
			},
		}
		if raw := r.Header.Get("Last-Event-ID"); raw != "" {
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
				opts.AfterSeq = v
			}
		}
		err := h.Conversations.SendStream(r.Context(), p, r.PathValue("conversation_id"), in.Content, opts)
		if err != nil && !errors.Is(err, run.ErrSubscribeDone) {
			if errors.Is(err, runtime.ErrMessageTooLong) {
				conversationError(w, r, err)
				return
			}
			b, _ := json.Marshal(streamError{Message: conversation.PublicError(err)})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
			flusher.Flush()
		}
		return
	}
	emit := func(d string) error {
		b, _ := json.Marshal(streamDelta{Delta: d})
		if _, err := fmt.Fprintf(w, "event: delta\ndata: %s\n\n", b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	m, u, err := h.Conversations.Send(r.Context(), p, r.PathValue("conversation_id"), in.Content, emit)
	if err != nil {
		if errors.Is(err, runtime.ErrMessageTooLong) {
			conversationError(w, r, err)
			return
		}
		b, _ := json.Marshal(streamError{Message: conversation.PublicError(err)})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
		flusher.Flush()
		return
	}
	b, _ := json.Marshal(streamDone{MessageID: m.ID, PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", b)
	flusher.Flush()
}

type waitingApprovalResponse struct {
	Code    string `json:"code" enum:"run_waiting_approval"`
	Message string `json:"message"`
	RunID   string `json:"run_id"`
}

func (e *waitingApprovalResponse) Error() string  { return e.Message }
func (e *waitingApprovalResponse) GetStatus() int { return http.StatusConflict }

func conversationError(w http.ResponseWriter, r *http.Request, err error) {
	frameworkhttp.WriteAPIError(w, mapConversationError(r.Context(), err))
}

func mapConversationError(ctx context.Context, err error) huma.StatusError {
	var waiting *conversation.WaitingApprovalError
	if errors.As(err, &waiting) {
		return &waitingApprovalResponse{Code: "run_waiting_approval", Message: waiting.Error(), RunID: waiting.RunID}
	}
	status := 500
	code := "conversation_unavailable"
	if errors.Is(err, runtime.ErrMessageTooLong) {
		status = http.StatusUnprocessableEntity
		code = "message_too_long"
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		status = 401
		code = "unauthenticated"
	}
	if errors.Is(err, definition.ErrNotFound) {
		status = 404
		code = "not_found"
	}
	if errors.Is(err, definition.ErrInvalid) {
		status = 422
		code = "invalid_request"
		if strings.Contains(err.Error(), "幂等键已被用于不同内容") {
			code = "idempotency_key_reused"
		}
	}
	if errors.Is(err, definition.ErrForbidden) {
		status = 403
		code = "forbidden"
	}
	if errors.Is(err, definition.ErrConflict) {
		status = 409
		code = "conflict"
	}
	return frameworkhttp.APIError(ctx, status, code, err)
}
