package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/tool"
)

var schemaToolResultRead = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["result_ref"],"properties":{"result_ref":{"type":"string","minLength":1,"maxLength":64},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":4000}}}`)

type ToolResultRead struct{ db *pgxpool.Pool }

func NewToolResultRead(db *pgxpool.Pool) *ToolResultRead { return &ToolResultRead{db: db} }

func (ToolResultRead) Spec() tool.Spec {
	return tool.Spec{
		Name: "tool_result_read", Version: "1", Title: "读取工具结果", Description: descToolResultRead,
		InputSchema: schemaToolResultRead, Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 16, Available: true,
	}
}

func (t *ToolResultRead) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	var in struct {
		ResultRef string `json:"result_ref"`
		Offset    int    `json:"offset"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_arguments", Message: "参数不是合法 JSON", Retryable: true}
	}
	if in.Limit <= 0 {
		in.Limit = 4000
	}
	var modelContent string
	var data []byte
	var total int
	err := t.db.QueryRow(ctx, `SELECT model_content, COALESCE(data::text,''), total_chars FROM tool_results WHERE id=$1 AND conversation_id=$2 AND owner_user_id=$3`,
		in.ResultRef, env.ConversationID, env.Principal.UserID).Scan(&modelContent, &data, &total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tool.Result{}, &tool.Error{Code: "not_found", Message: "未找到该结果引用", Retryable: false}
		}
		return tool.Result{}, err
	}
	text := modelContent
	if len(data) > 0 && text == "" {
		text = string(data)
	}
	runes := []rune(text)
	if in.Offset > len(runes) {
		in.Offset = len(runes)
	}
	end := in.Offset + in.Limit
	if end > len(runes) {
		end = len(runes)
	}
	slice := string(runes[in.Offset:end])
	return tool.Result{
		Data: map[string]any{
			"result_ref": in.ResultRef, "offset": in.Offset, "limit": in.Limit,
			"text": slice, "total_chars": len(runes), "has_more": end < len(runes),
		},
		Display: tool.Display{Summary: "读取工具结果片段"},
	}, nil
}
