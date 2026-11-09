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
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
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
	RunID            string    `json:"run_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Service struct {
	DB            *pgxpool.Pool
	Agents        definition.Service
	Model         provider.ChatModel
	Provider      provider.ChatProvider
	AllowedModels []string
	Memory        *memory.Service
	Runs          *run.RunService
	Registry      *tool.Registry
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
	query := `SELECT m.id,m.conversation_id,m.role,m.content,m.model,m.prompt_tokens,m.completion_tokens,m.latency_ms,m.status,m.error,COALESCE(m.run_id,''),m.created_at
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
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Model, &m.PromptTokens, &m.CompletionTokens, &m.LatencyMS, &m.Status, &m.Error, &m.RunID, &m.CreatedAt); err != nil {
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
	visible := make([]run.VisibleMessage, 0, len(history))
	previousUser := ""
	for _, m := range history {
		visible = append(visible, run.VisibleMessage{ID: m.ID, Role: m.Role, Content: m.Content, RunID: m.RunID, Status: "completed"})
		if m.Role == "user" {
			previousUser = m.Content
		}
	}
	limits, err := definition.ParseRuntimeLimits(version.Configuration.RuntimeLimits)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	runStore := run.NewPGStore(s.DB)
	projector := run.HistoryProjector{Store: runStore, Limits: limits}
	historyMessages, err := projector.Project(ctx, id, visible, policy.HistoryBudgetTokens)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	if s.Registry == nil {
		return Message{}, provider.Usage{}, fmt.Errorf("工具注册表未配置")
	}
	toolset, err := s.Registry.Resolve(tool.ResolveInput{
		Bindings: version.Configuration.Tools, MemoryPolicy: policy, Principal: p, CanApprove: false,
	})
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	builder := runtime.Builder{}
	memoryDetails := []runtime.Segment{}
	if s.Memory != nil {
		builder.Memory = &memory.Module{Service: s.Memory, Actor: p, AgentID: a.ID, Current: content + "\n" + previousUser, Policy: policy, Details: &memoryDetails}
	}
	if len(toolset.Entries) > 0 {
		builder.Tools = runtime.Tools{Toolset: toolset}
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
		PlatformRules: run.BuildPlatformRules("你是 xa 平台中的 Agent。遵守用户权限和隐私边界，清晰回答当前问题。"),
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
	if seg, err := run.ToolSchemaSegment(toolset); err == nil && seg.Name != "" {
		contextResult.Segments = append(contextResult.Segments, seg)
	}
	segments, err := json.Marshal(contextResult.Segments)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	now := time.Now().UTC()
	userID := "msg_" + uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user',$3,'completed',$4)`, userID, id, content, now); err != nil {
		return Message{}, provider.Usage{}, err
	}
	if s.Runs == nil {
		return Message{}, provider.Usage{}, fmt.Errorf("RunService 未配置")
	}
	assistantID, usage, _, callErr := s.Runs.ExecuteChat(ctx, tx, run.ChatStartInput{
		Principal: p, Conversation: run.ConversationRef{ID: id, Title: c.Title, AgentID: a.ID},
		Agent: a, Version: version, UserContent: content, UserMessageID: userID,
		InitialMessages: contextResult.Messages, Segments: segments, Toolset: toolset, Limits: limits,
		MemoryPolicy: policy, MemoryEnabled: policy.Enabled, AllowedModels: s.AllowedModels,
		EnableThinking: definition.ChatEnableThinking(version.Configuration),
	}, emit)
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelSave()
	var status, errText string
	if callErr != nil {
		_ = tx.Rollback(saveCtx)
		errText = PublicError(callErr)
		if errors.Is(ctx.Err(), context.Canceled) {
			errText = "已停止生成"
		}
		return Message{}, usage, callErr
	}
	if c.Title == "" {
		rn := []rune(content)
		if len(rn) > 30 {
			rn = rn[:30]
		}
		if _, err := tx.Exec(saveCtx, `UPDATE conversations SET title=$1,updated_at=now() WHERE id=$2`, string(rn), id); err != nil {
			return Message{}, usage, err
		}
	} else if _, err := tx.Exec(saveCtx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, id); err != nil {
		return Message{}, usage, err
	}
	if err := tx.Commit(saveCtx); err != nil {
		return Message{}, usage, err
	}
	if s.Memory != nil && policy.Enabled {
		enqueueTx, err := s.DB.Begin(saveCtx)
		if err != nil {
			return Message{}, usage, err
		}
		if err := s.Memory.EnqueueTx(saveCtx, enqueueTx, userID); err != nil {
			_ = enqueueTx.Rollback(saveCtx)
			return Message{}, usage, err
		}
		if err := enqueueTx.Commit(saveCtx); err != nil {
			return Message{}, usage, err
		}
	}
	var runID string
	_ = s.DB.QueryRow(context.WithoutCancel(ctx), `SELECT run_id FROM messages WHERE id=$1`, assistantID).Scan(&runID)
	var answer string
	var latencyMS int64
	_ = s.DB.QueryRow(context.WithoutCancel(ctx), `SELECT content,latency_ms,status,error FROM messages WHERE id=$1`, assistantID).Scan(&answer, &latencyMS, &status, &errText)
	return Message{
		ID: assistantID, ConversationID: id, Role: "assistant", Content: answer, Model: modelName,
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, LatencyMS: latencyMS,
		Status: status, Error: errText, RunID: runID,
	}, usage, callErr
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
