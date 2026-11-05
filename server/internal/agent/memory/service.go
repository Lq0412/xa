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
	Kind           string     `json:"kind"`
	Importance     int        `json:"importance"`
	Pinned         bool       `json:"pinned"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	SourceType     SourceType `json:"source_type"`
	SourceRef      *string    `json:"source_ref"`
	SupersedesID   *string    `json:"supersedes_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	Retention      float64    `json:"retention"`
	StrengthDays   float64    `json:"strength_days"`
	Sensitive      bool       `json:"sensitive"`
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
	Clock  Clock
}

// Filter 默认只返回 active；AgentID 是精确过滤，不包含共享层。
type Filter struct{ Scope, AgentID, Namespace, Status string }

const columns = `id,workspace_id,owner_user_id,scope,agent_id,conversation_id,namespace,subject_key,content,importance,pinned,status,source,source_ref,supersedes_id,created_at,updated_at,last_used_at,kind,sensitive,strength_days,retention`

func scan(row pgx.Row) (Item, error) {
	var m Item
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.OwnerUserID, &m.Scope, &m.AgentID, &m.ConversationID, &m.Namespace, &m.SubjectKey, &m.Content, &m.Importance, &m.Pinned, &m.Status, &m.Source, &m.SourceRef, &m.SupersedesID, &m.CreatedAt, &m.UpdatedAt, &m.LastUsedAt, &m.Kind, &m.Sensitive, &m.StrengthDays, &m.Retention)
	m.SourceType = SourceType(m.Source)
	if m.Source == "auto" || m.Source == "chat_extraction" {
		m.SourceType = SourceChat
		m.Source = "auto"
	}
	if m.Source == "user_edited" {
		m.SourceType = SourceUser
	}
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
	if (f.Status != "active" && f.Status != "superseded" && f.Status != "deleted" && f.Status != "archived") || (f.Scope != "" && f.Scope != "user" && f.Scope != "agent" && f.Scope != "session") {
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

// prohibitedContent 是抽取提示词之外的写入闸门，不保存高风险凭证。
var prohibitedContent = regexp.MustCompile(`(?i)(密码|password|api[_ -]?key|银行卡|身份证|\bsk-[a-z0-9]{12,}|\b[0-9]{16,19}\b|\b[0-9]{17}[0-9x]\b)`)

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
	if prohibitedContent.MatchString(next.Content) {
		return Item{}, definition.ErrInvalid
	}
	if next.Kind == "" {
		next.Kind = "fact"
	}
	actor := "agent"
	if next.Source == "user_edited" {
		actor = "user"
	}
	if next.SourceRef == nil && next.Source != "user_edited" {
		actor = "system"
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
	result, err := scan(tx.QueryRow(ctx, `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,agent_id,conversation_id,namespace,subject_key,content,kind,importance,pinned,status,source,source_ref,supersedes_id,embedding,sensitive) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'active',$13,$14,$15,$16::vector,$17) RETURNING `+columns, next.ID, p.WorkspaceID, p.UserID, next.Scope, next.AgentID, next.ConversationID, next.Namespace, next.SubjectKey, next.Content, next.Kind, next.Importance, next.Pinned, next.Source, next.SourceRef, next.SupersedesID, string(raw), next.Sensitive))
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

// Restore 将归档或删除的版本恢复为新的 active 版本，保留原始事件链。
func (s *Service) Restore(ctx context.Context, p auth.Principal, id string) (Item, error) {
	if err := authenticated(p); err != nil {
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
	if old.Status != "archived" && old.Status != "deleted" {
		return Item{}, definition.ErrConflict
	}
	vector, err := s.Model.Embed(ctx, old.Content)
	if err != nil {
		return Item{}, err
	}
	next := old
	next.Status = "active"
	next.Source = "user_edited"
	next.SourceRef = nil
	next.SupersedesID = &old.ID
	r, err := replace(ctx, tx, p, nil, next, vector)
	if err != nil {
		return Item{}, err
	}
	if err := event(ctx, tx, p, &r.ID, nil, "RESTORE", "user", &old, &r, ""); err != nil {
		return Item{}, err
	}
	return r, tx.Commit(ctx)
}

// AddInput 是显式新增记忆的请求模型。
type AddInput struct {
	Content    string `json:"content"`
	Namespace  string `json:"namespace"`
	SubjectKey string `json:"subject_key"`
	Kind       string `json:"kind"`
	Importance int    `json:"importance"`
	Pinned     bool   `json:"pinned"`
	Scope      string `json:"scope"`
	AgentID    string `json:"agent_id"`
}

// Add 校验作用域并以 user_edited 来源保存手动事实。
func (s *Service) Add(ctx context.Context, p auth.Principal, in AddInput) (Item, error) {
	if err := authenticated(p); err != nil {
		return Item{}, err
	}
	if in.Scope == "" {
		in.Scope = "user"
	}
	if in.Kind == "" {
		in.Kind = "fact"
	}
	if in.SubjectKey == "" {
		in.SubjectKey = in.Namespace
	}
	if !valid(EditInput{Content: in.Content, Namespace: in.Namespace, Importance: in.Importance}) || !namespacePattern.MatchString(in.SubjectKey) || (in.Kind != "fact" && in.Kind != "preference" && in.Kind != "episode" && in.Kind != "procedure") {
		return Item{}, definition.ErrInvalid
	}
	var aid *string
	if in.Scope == "agent" {
		if in.AgentID == "" {
			return Item{}, definition.ErrInvalid
		}
		aid = &in.AgentID
		if _, err := s.Agents.Get(ctx, p, in.AgentID); err != nil {
			return Item{}, err
		}
	} else if in.Scope != "user" {
		return Item{}, definition.ErrInvalid
	}
	v, err := s.Model.Embed(ctx, in.Content)
	if err != nil {
		return Item{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	item, err := replace(ctx, tx, p, nil, Item{Scope: in.Scope, AgentID: aid, Namespace: in.Namespace, SubjectKey: in.SubjectKey, Content: in.Content, Kind: in.Kind, Importance: in.Importance, Pinned: in.Pinned, Source: "user_edited"}, v)
	if err != nil {
		return Item{}, err
	}
	return item, tx.Commit(ctx)
}

// Stats 区分条目状态、活跃 namespace 和任务重试状态。
type Stats struct {
	Status         map[string]int `json:"status"`
	Namespace      map[string]int `json:"namespace"`
	Pending        int            `json:"pending"`
	Failed         int            `json:"failed"`
	Dead           int            `json:"dead"`
	LatestSummary  *time.Time     `json:"latest_summary_at,omitempty"`
	LatestMaintain *time.Time     `json:"latest_maintain_at,omitempty"`
}

// RetryJob 仅允许所有者重试 dead 任务，清空此前退避计数。
func (s *Service) RetryJob(ctx context.Context, p auth.Principal, id string) error {
	if err := authenticated(p); err != nil {
		return err
	}
	r, err := s.DB.Exec(ctx, `UPDATE memory_jobs SET status='pending',attempts=0,last_error='',next_run_at=now(),completed_at=NULL WHERE id=$1 AND owner_user_id=$2 AND status='dead'`, id, p.UserID)
	if err != nil {
		return err
	}
	if r.RowsAffected() == 0 {
		return definition.ErrNotFound
	}
	return nil
}

// Stats 不暴露其他用户的条目或后台任务。
func (s *Service) Stats(ctx context.Context, p auth.Principal) (Stats, error) {
	if err := authenticated(p); err != nil {
		return Stats{}, err
	}
	out := Stats{Status: map[string]int{}, Namespace: map[string]int{}}
	rows, err := s.DB.Query(ctx, `SELECT status,count(*) FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 GROUP BY status`, p.WorkspaceID, p.UserID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return out, err
		}
		out.Status[k] = n
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	rows, err = s.DB.Query(ctx, `SELECT namespace,count(*) FROM memory_items WHERE workspace_id=$1 AND owner_user_id=$2 AND status='active' GROUP BY namespace`, p.WorkspaceID, p.UserID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return out, err
		}
		out.Namespace[k] = n
	}
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='failed' OR (status='pending' AND attempts>0)),count(*) FILTER(WHERE status='dead') FROM memory_jobs WHERE owner_user_id=$1`, p.UserID).Scan(&out.Pending, &out.Failed, &out.Dead); err != nil {
		return out, err
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT (SELECT max(created_at) FROM conversation_summaries WHERE owner_user_id=$1),(SELECT max(completed_at) FROM memory_jobs WHERE owner_user_id=$1 AND kind='maintain' AND status='completed')`, p.UserID).Scan(&out.LatestSummary, &out.LatestMaintain); err != nil {
		return out, err
	}
	return out, nil
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
 UNION SELECT m.id,m.supersedes_id FROM memory_items m JOIN chain c ON m.id=c.supersedes_id OR m.supersedes_id=c.id OR EXISTS(SELECT 1 FROM memory_merge_sources x WHERE (x.memory_id=c.id AND x.source_id=m.id) OR (x.source_id=c.id AND x.memory_id=m.id)) WHERE m.workspace_id=$2 AND m.owner_user_id=$3)
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
