package builtin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/framework/redact"
	"linq-bs/server/internal/tool"
)

var schemaMemoryForget = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["memory_id","reason"],"properties":{"memory_id":{"type":"string","minLength":1,"maxLength":64},"reason":{"type":"string","minLength":1,"maxLength":200}}}`)

type MemoryForget struct{ svc *memory.Service }

// Preview 只读取当前用户在该 Agent 下可删除的 active 记忆。
func (m *MemoryForget) Preview(ctx context.Context, call tool.Call, env tool.Env) (tool.Display, error) {
	var in struct {
		MemoryID string `json:"memory_id"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Display{}, err
	}
	var content, namespace string
	err := m.svc.DB.QueryRow(ctx, `SELECT content,namespace FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3 AND status='active' AND (scope='user' OR (scope='agent' AND agent_id=$4))`, in.MemoryID, env.WorkspaceID, env.Principal.UserID, env.AgentID).Scan(&content, &namespace)
	if err == pgx.ErrNoRows {
		return tool.Display{}, &tool.Error{Code: "not_found", Message: "未找到可删除的记忆"}
	}
	if err != nil {
		return tool.Display{}, err
	}
	return tool.Display{Summary: "将删除这条记忆", Items: []tool.DisplayItem{{Label: "内容", Value: content}, {Label: "命名空间", Value: namespace}, {Label: "原因", Value: in.Reason}}}, nil
}

func NewMemoryForget(svc *memory.Service) *MemoryForget { return &MemoryForget{svc: svc} }

func (MemoryForget) Spec() tool.Spec {
	return tool.Spec{
		Name: "memory_forget", Version: "1", Title: "遗忘记忆", Description: descMemoryForget,
		InputSchema: schemaMemoryForget, Effect: tool.EffectDestructive, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalAlways, Overridable: true},
		DefaultTimeout: 20 * time.Second, MaxCallsPerRun: 4, Requires: []tool.Requirement{tool.RequireMemoryEnabled}, Available: true,
	}
}

func (m *MemoryForget) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	dbTx, err := m.svc.DB.Begin(ctx)
	if err != nil {
		return tool.Result{}, err
	}
	defer dbTx.Rollback(context.WithoutCancel(ctx))
	result, err := m.ExecuteTx(ctx, dbTx, call, env, struct{}{})
	if err != nil {
		return tool.Result{}, err
	}
	return result, dbTx.Commit(ctx)
}

func (m *MemoryForget) Prepare(ctx context.Context, call tool.Call, env tool.Env) (any, error) {
	_ = ctx
	_ = call
	_ = env
	return struct{}{}, nil
}

func (m *MemoryForget) ExecuteTx(ctx context.Context, tx pgx.Tx, call tool.Call, env tool.Env, _ any) (tool.Result, error) {
	return m.runTx(ctx, tx, call, env, nil)
}

func (m *MemoryForget) runTx(ctx context.Context, tx pgx.Tx, call tool.Call, env tool.Env, _ []float32) (tool.Result, error) {
	var in struct {
		MemoryID string `json:"memory_id"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	run := func(tx pgx.Tx) (tool.Result, error) {
		item, err := m.svc.ToolForgetTx(ctx, tx, env.Principal, env.AgentID, in.MemoryID, env.UserMessageID, in.Reason)
		if err != nil {
			return tool.Result{}, memory.ErrTool(err)
		}
		return tool.Result{
			Data:    map[string]any{"memory_id": item.ID, "status": "deleted"},
			Display: tool.Display{Summary: "已删除记忆", Items: []tool.DisplayItem{{Label: "内容", Value: trimRunes(redact.String(item.Content), 80)}}},
			Effects: []tool.Effect2{{Kind: "memory.delete", TargetID: item.ID, Attrs: map[string]string{"namespace": item.Namespace, "scope": item.Scope}}},
		}, nil
	}
	if tx != nil {
		return run(tx)
	}
	dbTx, err := m.svc.DB.Begin(ctx)
	if err != nil {
		return tool.Result{}, err
	}
	defer dbTx.Rollback(context.WithoutCancel(ctx))
	result, err := run(dbTx)
	if err != nil {
		return tool.Result{}, err
	}
	return result, dbTx.Commit(ctx)
}
