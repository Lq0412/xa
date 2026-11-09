package builtin

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/tool"
)

var schemaMemorySearch = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1,"maxLength":200},"top_k":{"type":"integer","minimum":1,"maximum":10},"namespace":{"type":"string","pattern":"^[a-z][a-z0-9_.-]{0,63}$"}}}`)

type MemorySearch struct{ svc *memory.Service }

func NewMemorySearch(svc *memory.Service) *MemorySearch { return &MemorySearch{svc: svc} }

func (MemorySearch) Spec() tool.Spec {
	return tool.Spec{
		Name: "memory_search", Version: "1", Title: "搜索记忆", Description: descMemorySearch,
		InputSchema: schemaMemorySearch, Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 15 * time.Second, MaxCallsPerRun: 16, Requires: []tool.Requirement{tool.RequireMemoryEnabled}, Available: true,
	}
}

func (m *MemorySearch) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	var in struct {
		Query     string `json:"query"`
		TopK      int    `json:"top_k"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	policy, err := m.svc.ParsePolicyFromAgent(ctx, env.Principal, env.AgentID)
	if err != nil {
		return tool.Result{}, memory.ErrTool(err)
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 10 {
		topK = 10
	}
	scored, err := m.svc.Recall(ctx, env.Principal, env.AgentID, in.Query, memory.RecallOptions{
		TopK: topK, MinRelevance: policy.MinRelevance, Namespace: in.Namespace, Touch: true,
	})
	if err != nil {
		return tool.Result{}, memory.ErrTool(err)
	}
	items, err := memory.ListScoredForTool(ctx, m.svc, env.Principal, env.AgentID, scored)
	if err != nil {
		return tool.Result{}, memory.ErrTool(err)
	}
	summary := "未找到相关记忆"
	if len(items) > 0 {
		summary = "找到 " + strconv.Itoa(len(items)) + " 条相关记忆"
	}
	return tool.Result{Data: map[string]any{"items": items}, Display: tool.Display{Summary: summary}}, nil
}
