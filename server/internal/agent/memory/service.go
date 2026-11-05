// Package memory 是记忆条目和审计事件的唯一写入边界。
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
)

// Item 的来源和取代指针不可原地修改，编辑生成新的 ID。
type Item struct {
	ID             string     `json:"id"`
	WorkspaceID    string     `json:"workspace_id"`
	OwnerUserID    string     `json:"owner_user_id"`
	Scope          string     `json:"scope"`
	AgentID        *string    `json:"agent_id"`
	ConversationID *string    `json:"conversation_id"`
	Namespace      string     `json:"namespace"`
	SubjectKey     string     `json:"subject_key"`
	Content        string     `json:"content"`
	Importance     int        `json:"importance"`
	Pinned         bool       `json:"pinned"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	SourceRef      *string    `json:"source_ref"`
	SupersedesID   *string    `json:"supersedes_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
}

// Event 保留操作前后快照；抽取失败没有 memory_id。
type Event struct {
	ID        int64           `json:"id"`
	MemoryID  *string         `json:"memory_id"`
	SourceRef *string         `json:"source_ref"`
	Op        string          `json:"op"`
	ActorType string          `json:"actor_type"`
	ActorID   string          `json:"actor_id"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"created_at"`
}

// Service 不暴露仓储写接口，模型凭证只存在供应商适配器中。
type Service struct {
	DB     *pgxpool.Pool
	Agents definition.Service
	Model  *provider.BailianModel
}

// Filter 默认只返回 active；AgentID 是精确过滤，不包含共享层。
type Filter struct{ Scope, AgentID, Namespace, Status string }

const columns = `id,workspace_id,owner_user_id,scope,agent_id,conversation_id,namespace,subject_key,content,importance,pinned,status,source,source_ref,supersedes_id,created_at,updated_at,last_used_at`

func scan(row pgx.Row) (Item, error) {
	var m Item
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.OwnerUserID, &m.Scope, &m.AgentID, &m.ConversationID, &m.Namespace, &m.SubjectKey, &m.Content, &m.Importance, &m.Pinned, &m.Status, &m.Source, &m.SourceRef, &m.SupersedesID, &m.CreatedAt, &m.UpdatedAt, &m.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = definition.ErrNotFound
	}
	return m, err
}

func authenticated(p auth.Principal) error {
	if p.UserID == "" || p.WorkspaceID == "" {
		return auth.ErrUnauthenticated
	}
	return nil
}

// List 始终使用认证主体过滤，管理员也没有跨用户读取能力。
func (s *Service) List(ctx context.Context, p auth.Principal, f Filter) ([]Item, error) {
	if err := authenticated(p); err != nil {
		return nil, err
	}
	if f.Status == "" {
		f.Status = "active"
	}
	if (f.Status != "active" && f.Status != "superseded" && f.Status != "deleted") || (f.Scope != "" && f.Scope != "user" && f.Scope != "agent" && f.Scope != "session") {
		return nil, definition.ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+columns+` FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND status=$3 AND ($4='' OR scope=$4) AND ($5='' OR agent_id=$5) AND ($6='' OR namespace=$6) ORDER BY pinned DESC,importance DESC,created_at DESC,id`, p.WorkspaceID, p.UserID, f.Status, f.Scope, f.AgentID, f.Namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EditInput 采用完整替换内容；旧条目的归属、来源消息和 subject_key 保留。
type EditInput struct {
	Content    string `json:"content"`
	Namespace  string `json:"namespace"`
	Importance int    `json:"importance"`
	Pinned     bool   `json:"pinned"`
}

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func valid(input EditInput) bool {
	return strings.TrimSpace(input.Content) != "" && len([]rune(input.Content)) <= 2000 && namespacePattern.MatchString(input.Namespace) && input.Importance >= 1 && input.Importance <= 10
}

func event(ctx context.Context, tx pgx.Tx, p auth.Principal, id, ref *string, op, actor string, before, after any, reason string) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	actorID := p.UserID
	if actor == "agent" && ref != nil {
		if err := tx.QueryRow(ctx, `SELECT c.agent_id FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=$1 AND c.workspace_id=$2 AND c.user_id=$3`, *ref, p.WorkspaceID, p.UserID).Scan(&actorID); err != nil {
			return err
		}
	}
	if actor == "system" {
		actorID = "memory-worker"
	}
	_, err = tx.Exec(ctx, `INSERT INTO memory_events(workspace_id,owner_user_id,memory_id,source_ref,op,actor_type,actor_id,before,after,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, p.WorkspaceID, p.UserID, id, ref, op, actor, actorID, b, a, reason)
	return err
}

// Edit 在网络请求前校验归属，再在事务中锁定旧条目防止并发覆盖。
func (s *Service) Edit(ctx context.Context, p auth.Principal, id string, input EditInput) (Item, error) {
	if err := authenticated(p); err != nil {
		return Item{}, err
	}
	if !valid(input) {
		return Item{}, definition.ErrInvalid
	}
	if _, err := scan(s.DB.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3`, id, p.WorkspaceID, p.UserID)); err != nil {
		return Item{}, err
	}
	vector, err := s.Model.Embed(ctx, input.Content)
	if err != nil {
		return Item{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	old, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3 FOR UPDATE`, id, p.WorkspaceID, p.UserID))
	if err != nil {
		return Item{}, err
	}
	next := old
	next.Content = input.Content
	next.Namespace = input.Namespace
	next.Importance = input.Importance
	next.Pinned = input.Pinned
	next.Source = "user_edited"
	next, err = replace(ctx, tx, p, &old, next, vector)
	if err != nil {
		return Item{}, err
	}
	return next, tx.Commit(ctx)
}

// replace 在同一事务内失效旧条目、插入新条目和两侧事件。
func replace(ctx context.Context, tx pgx.Tx, p auth.Principal, old *Item, next Item, vector []float32) (Item, error) {
	actor := "agent"
	if next.Source == "user_edited" {
		actor = "user"
	}
	if old != nil {
		if old.Status != "active" {
			return Item{}, definition.ErrConflict
		}
		if old.Source == "user_edited" && next.Source != "user_edited" {
			return Item{}, definition.ErrConflict
		}
		after, err := scan(tx.QueryRow(ctx, `UPDATE memory_items SET status='superseded',valid_to=now(),updated_at=now(),version=version+1 WHERE id=$1 RETURNING `+columns, old.ID))
		if err != nil {
			return Item{}, err
		}
		if err := event(ctx, tx, p, &old.ID, next.SourceRef, "SUPERSEDE", actor, old, after, ""); err != nil {
			return Item{}, err
		}
		next.SupersedesID = &old.ID
	}
	next.ID = "mem_" + uuid.NewString()
	raw, err := json.Marshal(vector)
	if err != nil {
		return Item{}, err
	}
	result, err := scan(tx.QueryRow(ctx, `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,agent_id,conversation_id,namespace,subject_key,content,importance,pinned,status,source,source_ref,supersedes_id,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'active',$12,$13,$14,$15::vector) RETURNING `+columns, next.ID, p.WorkspaceID, p.UserID, next.Scope, next.AgentID, next.ConversationID, next.Namespace, next.SubjectKey, next.Content, next.Importance, next.Pinned, next.Source, next.SourceRef, next.SupersedesID, string(raw)))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Item{}, definition.ErrConflict
		}
		return Item{}, err
	}
	op := "ADD"
	if old != nil {
		op = "UPDATE"
	}
	return result, event(ctx, tx, p, &result.ID, result.SourceRef, op, actor, old, result, "")
}

// Delete 保留取代链与审计，删除后的内容不会参与召回。
func (s *Service) Delete(ctx context.Context, p auth.Principal, id string) error {
	if err := authenticated(p); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	old, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3 FOR UPDATE`, id, p.WorkspaceID, p.UserID))
	if err != nil {
		return err
	}
	if old.Status == "deleted" {
		return tx.Commit(ctx)
	}
	after, err := scan(tx.QueryRow(ctx, `UPDATE memory_items SET status='deleted',valid_to=now(),updated_at=now(),version=version+1 WHERE id=$1 RETURNING `+columns, id))
	if err != nil {
		return err
	}
	if err := event(ctx, tx, p, &id, old.SourceRef, "DELETE", "user", old, after, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Events 沿双向取代链返回整条事实的历史，并在每层限制归属。
func (s *Service) Events(ctx context.Context, p auth.Principal, id string) ([]Event, error) {
	if err := authenticated(p); err != nil {
		return nil, err
	}
	if _, err := scan(s.DB.QueryRow(ctx, `SELECT `+columns+` FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3`, id, p.WorkspaceID, p.UserID)); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `WITH RECURSIVE chain AS (
 SELECT id,supersedes_id FROM memory_items WHERE id=$1 AND workspace_id=$2 AND owner_user_id=$3
 UNION SELECT m.id,m.supersedes_id FROM memory_items m JOIN chain c ON m.id=c.supersedes_id OR m.supersedes_id=c.id WHERE m.workspace_id=$2 AND m.owner_user_id=$3)
 SELECT e.id,e.memory_id,e.source_ref,e.op,e.actor_type,e.actor_id,e.before,e.after,e.reason,e.created_at FROM memory_events e WHERE e.workspace_id=$2 AND e.owner_user_id=$3 AND e.memory_id IN (SELECT id FROM chain) ORDER BY e.id`, id, p.WorkspaceID, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.MemoryID, &e.SourceRef, &e.Op, &e.ActorType, &e.ActorID, &e.Before, &e.After, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
