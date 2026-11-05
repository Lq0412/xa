package memory

import (
	"context"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
)

// Page 以不可变 id 为游标，避免优先级修改导致游标失效。
type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"next_cursor"`
}

// ListPage 在所有者范围内按不可变 ID 分页，all_visible 合并 user 和指定 Agent 层。
func (s *Service) ListPage(ctx context.Context, p auth.Principal, f Filter, cursor string, limit int) (Page, error) {
	if err := authenticated(p); err != nil {
		return Page{}, err
	}
	if limit < 1 || limit > 200 {
		return Page{}, definition.ErrInvalid
	}
	if f.Status == "" {
		f.Status = "active"
	}
	if f.Status != "active" && f.Status != "deleted" && f.Status != "superseded" && f.Status != "archived" {
		return Page{}, definition.ErrInvalid
	}
	if f.Scope != "" && f.Scope != "user" && f.Scope != "agent" && f.Scope != "session" && f.Scope != "all_visible" {
		return Page{}, definition.ErrInvalid
	}
	if f.Scope == "all_visible" {
		if f.AgentID == "" {
			return Page{}, definition.ErrInvalid
		}
		if _, err := s.Agents.Get(ctx, p, f.AgentID); err != nil {
			return Page{}, err
		}
	}
	if cursor != "" {
		var ok bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_items WHERE id=$1 AND owner_user_id=$2 AND workspace_id=$3)`, cursor, p.UserID, p.WorkspaceID).Scan(&ok); err != nil {
			return Page{}, err
		}
		if !ok {
			return Page{}, definition.ErrInvalid
		}
	}
	rows, err := s.DB.Query(ctx, `SELECT `+columns+` FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND status=$3 AND ($4='' OR scope=$4 OR ($4='all_visible' AND (scope='user' OR (scope='agent' AND agent_id=$5)))) AND ($4='all_visible' OR $5='' OR agent_id=$5) AND ($6='' OR namespace=$6) AND ($7='' OR id>$7) ORDER BY id LIMIT $8`, p.WorkspaceID, p.UserID, f.Status, f.Scope, f.AgentID, f.Namespace, cursor, limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	out := Page{Items: []Item{}}
	for rows.Next() {
		m, e := scan(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, m)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	return out, nil
}
