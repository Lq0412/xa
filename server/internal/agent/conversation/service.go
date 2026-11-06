package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
)

type Conversation struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	AgentID     string    `json:"agent_id"`
	Title       string    `json:"title"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Message struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversation_id"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	LatencyMS        int64     `json:"latency_ms"`
	Status           string    `json:"status"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Service struct {
	DB            *pgxpool.Pool
	Agents        definition.Service
	Model         provider.ChatModel
	AllowedModels []string
	Memory        *memory.Service
}

func (s *Service) Create(ctx context.Context, p auth.Principal, agentID string) (Conversation, error) {
	return s.GetOrCreate(ctx, p, agentID)
}

// GetOrCreate 保证一个用户和一个 Agent 只有一条对话。
func (s *Service) GetOrCreate(ctx context.Context, p auth.Principal, agentID string) (Conversation, error) {
	if p.UserID == "" {
		return Conversation{}, auth.ErrUnauthenticated
	}
	a, err := s.Agents.Get(ctx, p, agentID)
	if err != nil {
		return Conversation{}, err
	}
	if a.Status != definition.StatusActive {
		return Conversation{}, fmt.Errorf("Agent 已停用")
	}
	now := time.Now().UTC()
	c := Conversation{ID: "conv_" + uuid.NewString(), WorkspaceID: p.WorkspaceID, UserID: p.UserID, AgentID: a.ID, CreatedAt: now, UpdatedAt: now}
	err = s.DB.QueryRow(ctx, `
		INSERT INTO conversations(id,workspace_id,user_id,agent_id,title,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$6)
		ON CONFLICT (workspace_id,user_id,agent_id) DO UPDATE SET updated_at=conversations.updated_at
		RETURNING id,workspace_id,user_id,agent_id,title,created_at,updated_at`,
		c.ID, c.WorkspaceID, c.UserID, c.AgentID, "", now).
		Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}
func (s *Service) List(ctx context.Context, p auth.Principal) ([]Conversation, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,workspace_id,user_id,agent_id,title,created_at,updated_at FROM conversations WHERE workspace_id=$1 AND user_id=$2 ORDER BY updated_at DESC`, p.WorkspaceID, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Conversation, 0)
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type MessagePage struct {
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"has_more"`
	Before   string    `json:"before,omitempty"`
}

// Summary 只向会话所有者返回最新摘要及覆盖范围。
func (s *Service) Summary(ctx context.Context, p auth.Principal, id string) (map[string]any, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1 AND user_id=$2 AND workspace_id=$3)`, id, p.UserID, p.WorkspaceID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, definition.ErrNotFound
	}
	var out map[string]any
	var raw []byte
	if err := s.DB.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT id,conversation_id,content,covers_from_message_id,covers_until_message_id,message_count,estimated_tokens,version,created_at FROM conversation_summaries WHERE conversation_id=$1 AND owner_user_id=$2 ORDER BY version DESC LIMIT 1) x`, id, p.UserID).Scan(&raw); err != nil {
		if err == pgx.ErrNoRows {
			return map[string]any{"summary": nil}, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) Messages(ctx context.Context, p auth.Principal, id string, before string, limit int) (MessagePage, error) {
	if limit <= 0 || limit > 100 {
		return MessagePage{}, definition.ErrInvalid
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE id=$1 AND workspace_id=$2 AND user_id=$3)`, id, p.WorkspaceID, p.UserID).Scan(&exists); err != nil {
		return MessagePage{}, err
	}
	if !exists {
		return MessagePage{}, definition.ErrNotFound
	}
	query := `SELECT m.id,m.conversation_id,m.role,m.content,m.model,m.prompt_tokens,m.completion_tokens,m.latency_ms,m.status,m.error,m.created_at
		FROM messages m JOIN conversations c ON c.id=m.conversation_id
		WHERE m.conversation_id=$1 AND c.workspace_id=$2 AND c.user_id=$3`
	args := []any{id, p.WorkspaceID, p.UserID}
	if before != "" {
		var cursorExists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE id=$1 AND conversation_id=$2)`, before, id).Scan(&cursorExists); err != nil {
			return MessagePage{}, err
		}
		if !cursorExists {
			return MessagePage{}, definition.ErrInvalid
		}
		query += " AND (m.created_at,m.id) < (SELECT created_at,id FROM messages WHERE id=$4 AND conversation_id=$1)"
		args = append(args, before)
	}
	query += fmt.Sprintf(" ORDER BY m.created_at DESC,m.id DESC LIMIT %d", limit+1)
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return MessagePage{}, err
	}
	defer rows.Close()
	out := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Model, &m.PromptTokens, &m.CompletionTokens, &m.LatencyMS, &m.Status, &m.Error, &m.CreatedAt); err != nil {
			return MessagePage{}, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return MessagePage{}, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	page := MessagePage{Messages: out, HasMore: hasMore}
	if hasMore && len(out) > 0 {
		page.Before = out[0].ID
	}
	return page, nil
}

func (s *Service) Send(ctx context.Context, p auth.Principal, id, content string, emit func(string) error) (Message, provider.Usage, error) {
	if strings.TrimSpace(content) == "" {
		return Message{}, provider.Usage{}, fmt.Errorf("消息不能为空")
	}
	var c Conversation
	// 事务级 advisory lock 序列化同一对话的轮次，避免多个标签页交叉拼接历史。
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, id).Scan(&locked); err != nil {
		return Message{}, provider.Usage{}, err
	}
	if !locked {
		return Message{}, provider.Usage{}, fmt.Errorf("对话正在生成回复: %w", definition.ErrConflict)
	}
	if err := s.DB.QueryRow(ctx, `SELECT id,workspace_id,user_id,agent_id,title,created_at,updated_at FROM conversations WHERE id=$1 AND workspace_id=$2 AND user_id=$3`, id, p.WorkspaceID, p.UserID).Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return Message{}, provider.Usage{}, fmt.Errorf("会话不存在")
		}
		return Message{}, provider.Usage{}, err
	}
	var version definition.AgentVersion
	a, err := s.Agents.Get(ctx, p, c.AgentID)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	if a.Status != definition.StatusActive {
		return Message{}, provider.Usage{}, definition.ErrForbidden
	}
	version, err = s.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	policy, err := definition.ParseMemoryPolicy(version.Configuration.MemoryPolicy)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	windowService := s.Memory
	if windowService == nil {
		windowService = &memory.Service{DB: s.DB}
	}
	if err := runtime.ValidateCurrent(content); err != nil {
		return Message{}, provider.Usage{}, err
	}
	window, err := windowService.Window(ctx, p, id, policy.HistoryBudgetTokens)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	history := window.Messages
	historyMessages := make([]provider.Message, 0, len(history))
	previousUser := ""
	for _, m := range history {
		historyMessages = append(historyMessages, provider.Message{Role: m.Role, Content: m.Content})
		if m.Role == "user" {
			previousUser = m.Content
		}
	}
	builder := runtime.Builder{}
	memoryDetails := []runtime.Segment{}
	if s.Memory != nil {
		builder.Memory = &memory.Module{Service: s.Memory, Actor: p, AgentID: a.ID, Current: content + "\n" + previousUser, Policy: policy, Details: &memoryDetails}
	}
	var sessionSummary string
	var summaryID string
	var summaryVersion int
	if policy.Enabled {
		err = s.DB.QueryRow(ctx, `SELECT id,version,content FROM conversation_summaries WHERE conversation_id=$1 AND owner_user_id=$2 ORDER BY version DESC LIMIT 1`, id, p.UserID).Scan(&summaryID, &summaryVersion, &sessionSummary)
		if err != nil && err != pgx.ErrNoRows {
			return Message{}, provider.Usage{}, err
		}
		if len(history) > 0 && s.Memory != nil {
			if err = s.Memory.EnqueueSummaryTx(ctx, tx, p, id, history[0].ID, policy.SummaryTriggerTokens); err != nil {
				return Message{}, provider.Usage{}, err
			}
		}
	}
	contextResult, err := builder.Build(ctx, runtime.Input{
		PlatformRules: "你是 xa 平台中的 Agent。遵守用户权限和隐私边界，清晰回答当前问题。",
		SoulMD:        version.Configuration.SoulMD, History: historyMessages, Current: content, SessionSummary: sessionSummary,
	})
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	if window.Details.OverBudget {
		contextResult.Segments = slices.Insert(contextResult.Segments, len(contextResult.Segments)-1, window.Details)
	}
	for i := range contextResult.Segments {
		seg := &contextResult.Segments[i]
		if seg.Name == "session_summary" {
			seg.SummaryID = summaryID
			seg.SummaryVersion = summaryVersion
		}
		if seg.Name == "history" && len(history) > 0 {
			seg.FromMessageID = history[0].ID
			seg.UntilMessageID = history[len(history)-1].ID
		}
	}
	modelName := version.Configuration.Model
	if !slices.Contains(s.AllowedModels, modelName) {
		return Message{}, provider.Usage{}, fmt.Errorf("不支持的模型 %s: %w", modelName, definition.ErrInvalid)
	}
	segments, err := json.Marshal(contextResult.Segments)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	now := time.Now().UTC()
	userID := "msg_" + uuid.NewString()
	if _, err := s.DB.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user',$3,'completed',$4)`, userID, id, content, now); err != nil {
		return Message{}, provider.Usage{}, err
	}
	var answer strings.Builder
	began := time.Now()
	usage, callErr := s.Model.Stream(ctx, modelName, contextResult.Messages, func(d string) error { answer.WriteString(d); return emit(d) })
	latency := time.Since(began).Milliseconds()
	status := "completed"
	errText := ""
	if callErr != nil {
		status = "failed"
		errText = callErr.Error()
		if errors.Is(ctx.Err(), context.Canceled) {
			errText = "已停止生成"
		}
	}
	// 浏览器停止接收时请求 context 已取消；最终记录使用有时限的独立 context 保存。
	// 沿用 failed 状态，error 明确区分主动停止，刷新后仍能查看已经生成的部分。
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelSave()
	assistantID := "msg_" + uuid.NewString()
	_, dbErr := tx.Exec(saveCtx, `INSERT INTO messages(id,conversation_id,role,content,model,prompt_tokens,completion_tokens,latency_ms,status,error,created_at,context_segments,agent_version_id) VALUES($1,$2,'assistant',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, assistantID, id, answer.String(), modelName, usage.PromptTokens, usage.CompletionTokens, latency, status, errText, time.Now().UTC(), segments, version.ID)
	if dbErr != nil {
		return Message{}, usage, dbErr
	}
	if c.Title == "" {
		r := []rune(content)
		if len(r) > 30 {
			r = r[:30]
		}
		_, dbErr = tx.Exec(saveCtx, `UPDATE conversations SET title=$1,updated_at=now() WHERE id=$2`, string(r), id)
	} else {
		_, dbErr = tx.Exec(saveCtx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, id)
	}
	if dbErr != nil {
		return Message{}, usage, dbErr
	}
	if s.Memory != nil && policy.Enabled && callErr == nil {
		if err := s.Memory.EnqueueTx(saveCtx, tx, userID); err != nil {
			return Message{}, usage, err
		}
	}
	if err := tx.Commit(saveCtx); err != nil {
		return Message{}, usage, err
	}
	return Message{ID: assistantID, ConversationID: id, Role: "assistant", Content: answer.String(), Model: modelName, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, LatencyMS: latency, Status: status, Error: errText}, usage, callErr
}

func (s *Service) loadMessages(ctx context.Context, p auth.Principal, id string, limit int) ([]Message, error) {
	page, err := s.Messages(ctx, p, id, "", limit)
	return page.Messages, err
}

func (s *Service) loadMessagesBudget(ctx context.Context, p auth.Principal, id string, budget int) ([]Message, error) {
	window, err := (&memory.Service{DB: s.DB}).Window(ctx, p, id, budget)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(window.Messages))
	for _, m := range window.Messages {
		out = append(out, Message{ID: m.ID, Role: m.Role, Content: m.Content})
	}
	return out, nil
}
