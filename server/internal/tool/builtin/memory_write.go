package builtin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/tool"
)

var schemaMemoryWrite = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["content","namespace"],"properties":{"content":{"type":"string","minLength":1,"maxLength":500},"namespace":{"type":"string","pattern":"^[a-z][a-z0-9_.-]{0,63}$"},"subject_key":{"type":"string","pattern":"^[a-z][a-z0-9_.-]{0,63}$"},"kind":{"type":"string","enum":["fact","preference","episode","procedure"]},"importance":{"type":"integer","minimum":1,"maximum":10}}}`)

type MemoryWrite struct{ svc *memory.Service }

func NewMemoryWrite(svc *memory.Service) *MemoryWrite { return &MemoryWrite{svc: svc} }

func (MemoryWrite) Spec() tool.Spec {
	return tool.Spec{
		Name: "memory_write", Version: "1", Title: "写入记忆", Description: descMemoryWrite,
		InputSchema: schemaMemoryWrite, Effect: tool.EffectWrite, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 20 * time.Second, MaxCallsPerRun: 8, Requires: []tool.Requirement{tool.RequireMemoryEnabled}, Available: true,
	}
}

func (m *MemoryWrite) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	return m.run(ctx, call, env, nil)
}

func (m *MemoryWrite) Prepare(ctx context.Context, call tool.Call, env tool.Env) (any, error) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return nil, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	return m.svc.ToolWritePrepare(ctx, in.Content)
}

func (m *MemoryWrite) ExecuteTx(ctx context.Context, tx pgx.Tx, call tool.Call, env tool.Env, prepared any) (tool.Result, error) {
	vector, _ := prepared.([]float32)
	return m.runTx(ctx, tx, call, env, vector)
}

func (m *MemoryWrite) run(ctx context.Context, call tool.Call, env tool.Env, tx pgx.Tx) (tool.Result, error) {
	vector, err := m.Prepare(ctx, call, env)
	if err != nil {
		return tool.Result{}, err
	}
	v, _ := vector.([]float32)
	if tx != nil {
		return m.runTx(ctx, tx, call, env, v)
	}
	dbTx, err := m.svc.DB.Begin(ctx)
	if err != nil {
		return tool.Result{}, err
	}
	defer dbTx.Rollback(context.WithoutCancel(ctx))
	result, err := m.runTx(ctx, dbTx, call, env, v)
	if err != nil {
		return tool.Result{}, err
	}
	return result, dbTx.Commit(ctx)
}

func (m *MemoryWrite) runTx(ctx context.Context, tx pgx.Tx, call tool.Call, env tool.Env, vector []float32) (tool.Result, error) {
	var in struct {
		Content    string `json:"content"`
		Namespace  string `json:"namespace"`
		SubjectKey string `json:"subject_key"`
		Kind       string `json:"kind"`
		Importance int    `json:"importance"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	item, op, err := m.svc.ToolWriteTx(ctx, tx, env.Principal, memory.ToolWriteInput{
		AgentID: env.AgentID, IsPrimary: env.IsPrimaryAgent, SourceRef: env.UserMessageID,
		Content: in.Content, Namespace: in.Namespace, SubjectKey: in.SubjectKey, Kind: in.Kind, Importance: in.Importance,
	}, vector)
	if err != nil {
		return tool.Result{}, memory.ErrTool(err)
	}
	effects := []tool.Effect2{}
	if op != "noop" {
		effects = append(effects, tool.Effect2{Kind: "memory." + op, TargetID: item.ID, Attrs: map[string]string{"namespace": item.Namespace, "scope": item.Scope}})
	}
	opLabel := map[string]string{"add": "添加", "update": "更新", "noop": "保持"}[op]
	if opLabel == "" {
		opLabel = op
	}
	return tool.Result{
		Data:    map[string]any{"memory_id": item.ID, "op": op, "namespace": item.Namespace, "scope": item.Scope},
		Display: tool.Display{Summary: "记忆已" + opLabel, Items: []tool.DisplayItem{{Label: "内容", Value: trimRunes(item.Content, 80)}}},
		Effects: effects,
	}, nil
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
