package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/rag"
	"linq-bs/server/internal/tool"
)

var schemaKbSearch = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1,"maxLength":200},"top_k":{"type":"integer","minimum":1,"maximum":20},"collection_ids":{"type":"array","items":{"type":"string"}}}}`)

// KbSearch 只使用运行环境提供的 Agent 和不可变版本，模型不能覆盖它们。
type KbSearch struct{ search *rag.Searcher }

// NewKbSearch 注入与摄取使用相同存储和 embedding 模型的检索服务。
func NewKbSearch(search *rag.Searcher) *KbSearch { return &KbSearch{search: search} }

// Spec 在存在启用的知识库绑定时开放按需检索；仍可由工具配置显式关闭。
func (KbSearch) Spec() tool.Spec {
	return tool.Spec{
		Name: "kb_search", Version: "1", Title: "搜索知识库", Description: descKbSearch,
		InputSchema: schemaKbSearch, Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 30 * time.Second, MaxCallsPerRun: 8,
		Requires: []tool.Requirement{tool.RequireKnowledgeBound}, Available: true,
	}
}

// Execute 的结构化结果由现有工具循环写入运行记录，保留命中快照用于引用核对。
func (k *KbSearch) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	var in rag.SearchInput
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	result, err := k.search.Search(ctx, env.Principal, env.AgentID, env.AgentVersionID, in)
	if err != nil {
		var f *rag.Fault
		if errors.As(err, &f) {
			return tool.Result{}, &tool.Error{Code: f.Code, Message: f.Message, Retryable: f.Status >= 500 || f.Code == "invalid_arguments"}
		}
		if errors.Is(err, definition.ErrNotFound) || errors.Is(err, definition.ErrForbidden) {
			return tool.Result{}, &tool.Error{Code: "knowledge_unavailable", Message: "Agent 或配置版本不可访问"}
		}
		return tool.Result{}, &tool.Error{Code: "knowledge_unavailable", Message: "知识库检索暂时不可用，请稍后重试", Retryable: true}
	}
	summary := "未找到可用的相关资料"
	if len(result.Items) > 0 {
		summary = "找到 " + strconv.Itoa(len(result.Items)) + " 条相关资料"
	}
	return tool.Result{Data: result, Display: tool.Display{Summary: summary}}, nil
}
