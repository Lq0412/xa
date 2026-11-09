package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
)

// ToolWriteInput 描述工具写入一条记忆。
type ToolWriteInput struct {
	AgentID    string
	IsPrimary  bool
	SourceRef  string
	Content    string
	Namespace  string
	SubjectKey string
	Kind       string
	Importance int
}

// ToolWritePrepare 在事务外生成向量。
func (s *Service) ToolWritePrepare(ctx context.Context, content string) ([]float32, error) {
	if strings.TrimSpace(content) == "" {
		return nil, definition.ErrInvalid
	}
	return s.Model.Embed(ctx, content)
}

// ToolWriteTx 在 runtime 提供的事务内写入或更新记忆。
func (s *Service) ToolWriteTx(ctx context.Context, tx pgx.Tx, p auth.Principal, in ToolWriteInput, vector []float32) (Item, string, error) {
	if err := authenticated(p); err != nil {
		return Item{}, "", err
	}
	if len([]rune(in.Content)) < 1 || len([]rune(in.Content)) > 500 || !namespacePattern.MatchString(in.Namespace) {
		return Item{}, "", toolError("schema_violation", "内容或 namespace 无效", true)
	}
	if in.Importance < 1 || in.Importance > 10 {
		in.Importance = 6
	}
	if in.Kind == "" {
		in.Kind = "fact"
	}
	if prohibitedContent.MatchString(in.Content) {
		return Item{}, "", toolError("content_prohibited", "内容包含不允许保存的敏感信息", false)
	}
	subject := in.SubjectKey
	if subject == "" {
		subject = in.Namespace
	}
	if !namespacePattern.MatchString(subject) {
		return Item{}, "", toolError("schema_violation", "subject_key 无效", true)
	}
	scope := "agent"
	var aid *string
	if in.IsPrimary {
		scope = "user"
	} else {
		aid = &in.AgentID
	}
	var old *Item
	candidate, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND scope=$3 AND agent_id IS NOT DISTINCT FROM $4 AND conversation_id IS NULL AND namespace=$5 AND subject_key=$6 AND status='active' FOR UPDATE`, p.WorkspaceID, p.UserID, scope, aid, in.Namespace, subject))
	if err == nil {
		old = &candidate
		if old.Content == in.Content {
			return candidate, "noop", nil
		}
		if old.Source == "user_edited" {
			return Item{}, "", toolError("conflict_user_edited", "该事实由用户手动维护，请告知用户自行修改", false)
		}
	} else if !errors.Is(err, definition.ErrNotFound) && !errors.Is(err, pgx.ErrNoRows) {
		return Item{}, "", err
	}
	ref := in.SourceRef
	next := Item{
		Scope: scope, AgentID: aid, Namespace: in.Namespace, SubjectKey: subject,
		Content: in.Content, Kind: in.Kind, Importance: in.Importance, Source: "tool",
	}
	if ref != "" {
		next.SourceRef = &ref
	}
	item, err := replace(ctx, tx, p, old, next, vector)
	if err != nil {
		if errors.Is(err, definition.ErrConflict) {
			return Item{}, "", toolError("conflict_user_edited", "该事实由用户手动维护，请告知用户自行修改", false)
		}
		return Item{}, "", err
	}
	op := "add"
	if old != nil {
		op = "update"
	}
	return item, op, nil
}

// ToolForgetTx 将一条 active 记忆标记为 deleted。
func (s *Service) ToolForgetTx(ctx context.Context, tx pgx.Tx, p auth.Principal, agentID, memoryID, sourceRef, reason string) (Item, error) {
	if err := authenticated(p); err != nil {
		return Item{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 200 {
		return Item{}, toolError("schema_violation", "reason 长度须在 1–200", true)
	}
	old, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3 AND status='active' AND (scope='user' OR (scope='agent' AND agent_id=$4)) FOR UPDATE`, memoryID, p.WorkspaceID, p.UserID, agentID))
	if err != nil {
		if errors.Is(err, definition.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return Item{}, toolError("not_found", "未找到可删除的记忆", false)
		}
		return Item{}, err
	}
	after, err := scan(tx.QueryRow(ctx, `UPDATE memory_items SET status='deleted',valid_to=now(),updated_at=now(),version=version+1 WHERE id=$1 RETURNING `+columns, memoryID))
	if err != nil {
		return Item{}, err
	}
	ref := sourceRef
	if ref == "" {
		ref = deref(old.SourceRef)
	}
	if err := event(ctx, tx, p, &memoryID, &ref, "DELETE", "agent", old, after, truncateReason(reason)); err != nil {
		return Item{}, err
	}
	if err := s.enqueueSummaryRewrite(ctx, tx, p, old); err != nil {
		return Item{}, err
	}
	return after, nil
}

func toolError(code, message string, retryable bool) error {
	return &tool.Error{Code: code, Message: message, Retryable: retryable}
}

func truncateReason(reason string) string {
	runes := []rune(reason)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return reason
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ListScoredForTool 把召回结果扩展为工具返回字段。
func ListScoredForTool(ctx context.Context, s *Service, p auth.Principal, agentID string, scored []Scored) ([]map[string]any, error) {
	if len(scored) == 0 {
		return []map[string]any{}, nil
	}
	ids := make([]string, len(scored))
	scoreByID := map[string]float64{}
	for i, row := range scored {
		ids[i] = row.ID
		scoreByID[row.ID] = row.Score
	}
	rows, err := s.DB.Query(ctx, `SELECT `+columns+` FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND id=ANY($3) AND sensitive=false`, p.WorkspaceID, p.UserID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]Item{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items[m.ID] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(scored))
	for _, row := range scored {
		m, ok := items[row.ID]
		if !ok || m.Sensitive {
			continue
		}
		out = append(out, map[string]any{
			"id": row.ID, "content": m.Content, "namespace": m.Namespace, "subject_key": m.SubjectKey,
			"kind": m.Kind, "scope": m.Scope, "score": scoreByID[row.ID], "updated_at": m.UpdatedAt,
		})
	}
	return out, nil
}

// ErrTool 将 memory 层业务错误转为 tool.Error。
func ErrTool(err error) error {
	if err == nil {
		return nil
	}
	var te *tool.Error
	if errors.As(err, &te) {
		return te
	}
	if errors.Is(err, definition.ErrNotFound) {
		return toolError("not_found", "未找到资源", false)
	}
	if errors.Is(err, definition.ErrForbidden) {
		return toolError("permission_denied", "无权限执行该操作", false)
	}
	return fmt.Errorf("%w", err)
}
