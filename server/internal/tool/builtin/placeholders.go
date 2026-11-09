package builtin

import (
	"context"
	"encoding/json"
	"time"

	"linq-bs/server/internal/tool"
)

var schemaKbSearch = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1,"maxLength":200},"top_k":{"type":"integer","minimum":1,"maximum":20},"collection_ids":{"type":"array","items":{"type":"string"}}}}`)
var schemaAskAgent = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["agent_id","task"],"properties":{"agent_id":{"type":"string","minLength":1,"maxLength":64},"task":{"type":"string","minLength":1,"maxLength":2000},"context":{"type":"string","maxLength":4000}}}`)

type KbSearch struct{}

func (KbSearch) Spec() tool.Spec {
	return tool.Spec{
		Name: "kb_search", Version: "1", Title: "搜索知识库", Description: descKbSearch,
		InputSchema: schemaKbSearch, Effect: tool.EffectRead, DefaultEnabled: false,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 30 * time.Second, MaxCallsPerRun: 8, Available: false,
	}
}

func (KbSearch) Execute(context.Context, tool.Call, tool.Env) (tool.Result, error) {
	return tool.Result{}, &tool.Error{Code: "tool_unavailable", Message: "知识库尚未上线", Retryable: false}
}

type AskAgent struct{}

func (AskAgent) Spec() tool.Spec {
	return tool.Spec{
		Name: "ask_agent", Version: "1", Title: "委派 Agent", Description: descAskAgent,
		InputSchema: schemaAskAgent, Effect: tool.EffectWrite, DefaultEnabled: false,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalAlways, Overridable: false},
		DefaultTimeout: 60 * time.Second, MaxCallsPerRun: 2, Available: false,
	}
}

func (AskAgent) Execute(context.Context, tool.Call, tool.Env) (tool.Result, error) {
	return tool.Result{}, &tool.Error{Code: "tool_unavailable", Message: "子代理尚未上线", Retryable: false}
}
