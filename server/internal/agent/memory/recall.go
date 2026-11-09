package memory

import (
	"context"
	"encoding/json"
	"slices"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
)

// RecallOptions 控制向量混合召回。
type RecallOptions struct {
	TopK         int
	MinRelevance float64
	Namespace    string
	ExcludeIDs   []string
	Touch        bool
}

// Scored 是一条召回候选及其混合分数。
type Scored struct {
	ID      string
	Content string
	Score   float64
}

// Recall 抽取自 Module.Context 的混合召回；Touch 为 true 时更新 last_used_at 与衰减参数。
func (s *Service) Recall(ctx context.Context, p auth.Principal, agentID, query string, o RecallOptions) ([]Scored, error) {
	if err := authenticated(p); err != nil {
		return nil, err
	}
	if _, err := s.Agents.Get(ctx, p, agentID); err != nil {
		return nil, err
	}
	topK := o.TopK
	if topK <= 0 {
		topK = 5
	}
	minRel := o.MinRelevance
	exclude := o.ExcludeIDs
	if exclude == nil {
		exclude = []string{}
	}
	vector, err := s.Model.Embed(ctx, runtime.TruncateTokens(query, 6000))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(vector)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = tx.Exec(ctx, `SET LOCAL hnsw.iterative_scan='relaxed_order'`); err != nil {
		return nil, err
	}
	sql := `WITH candidates AS MATERIALIZED (
SELECT id,content,importance,embedding,CASE WHEN pinned OR source='user_edited' THEN 1 ELSE exp(-greatest(0,extract(epoch FROM ($7::timestamptz-COALESCE(last_used_at,created_at))))/(strength_days*86400.0)) END AS retention
FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND status='active' AND (scope='user' OR (scope='agent' AND agent_id=$3))`
	args := []any{p.WorkspaceID, p.UserID, agentID, string(raw), minRel, topK, s.now(), exclude}
	if o.Namespace != "" {
		sql += ` AND namespace=$9`
		args = append(args, o.Namespace)
	}
	sql += ` ORDER BY embedding <=> $4::vector LIMIT 50)
SELECT id,content,(1-(embedding <=> $4::vector))*0.6+(importance/10.0)*0.2+retention*0.2 AS score FROM candidates WHERE 1-(embedding <=> $4::vector) >= $5 AND NOT(id=ANY($8::text[])) ORDER BY score DESC,id LIMIT $6`
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out := []Scored{}
	ids := []string{}
	for rows.Next() {
		var id, content string
		var score float64
		if err := rows.Scan(&id, &content, &score); err != nil {
			rows.Close()
			return nil, err
		}
		if slices.Contains(exclude, id) {
			continue
		}
		out = append(out, Scored{ID: id, Content: content, Score: score})
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if o.Touch && len(ids) > 0 {
		if _, err = tx.Exec(ctx, `UPDATE memory_items SET last_used_at=$4,strength_days=least(strength_days*1.5,365),retention=1 WHERE workspace_id=$1 AND owner_user_id=$2 AND status='active' AND id=ANY($3)`, p.WorkspaceID, p.UserID, ids, s.now()); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// RecallTouchIDs 在已选定条目上执行与 Context 相同的衰减更新（供 Builder 注入路径使用）。
func (s *Service) RecallTouchIDs(ctx context.Context, p auth.Principal, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.DB.Exec(ctx, `UPDATE memory_items SET last_used_at=$4,strength_days=least(strength_days*1.5,365),retention=1 WHERE workspace_id=$1 AND owner_user_id=$2 AND status='active' AND id=ANY($3)`, p.WorkspaceID, p.UserID, ids, s.now())
	return err
}

// visibleItem 校验记忆对当前用户与 Agent 可见。
func (s *Service) visibleItem(ctx context.Context, p auth.Principal, agentID, memoryID string) (Item, error) {
	row := s.DB.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3 AND status='active' AND (scope='user' OR (scope='agent' AND agent_id=$4))`, memoryID, p.WorkspaceID, p.UserID, agentID)
	return scan(row)
}

// ParsePolicyFromAgent 读取 Agent 当前版本的记忆策略。
func (s *Service) ParsePolicyFromAgent(ctx context.Context, p auth.Principal, agentID string) (definition.MemoryPolicy, error) {
	a, err := s.Agents.Get(ctx, p, agentID)
	if err != nil {
		return definition.MemoryPolicy{}, err
	}
	v, err := s.Agents.GetVersion(ctx, p, agentID, a.CurrentVersionID)
	if err != nil {
		return definition.MemoryPolicy{}, err
	}
	return definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
}
