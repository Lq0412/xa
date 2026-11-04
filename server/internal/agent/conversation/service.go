package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/definition"
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
	DB        *pgxpool.Pool
	Agents    definition.Service
	Model     provider.ChatModel
	ModelName string
}

func (s *Service) Create(ctx context.Context, p auth.Principal, agentID string) (Conversation, error) {
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
	_, err = s.DB.Exec(ctx, `INSERT INTO conversations(id,workspace_id,user_id,agent_id,title,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$6)`, c.ID, c.WorkspaceID, c.UserID, c.AgentID, "", now)
	return c, err
}
func (s *Service) List(ctx context.Context, p auth.Principal) ([]Conversation, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,workspace_id,user_id,agent_id,title,created_at,updated_at FROM conversations WHERE workspace_id=$1 AND user_id=$2 ORDER BY updated_at DESC`, p.WorkspaceID, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) Messages(ctx context.Context, p auth.Principal, id string) ([]Message, error) {
	rows, err := s.DB.Query(ctx, `SELECT m.id,m.conversation_id,m.role,m.content,m.model,m.prompt_tokens,m.completion_tokens,m.latency_ms,m.status,m.error,m.created_at FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.conversation_id=$1 AND c.workspace_id=$2 AND c.user_id=$3 ORDER BY m.created_at`, id, p.WorkspaceID, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Model, &m.PromptTokens, &m.CompletionTokens, &m.LatencyMS, &m.Status, &m.Error, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) Send(ctx context.Context, p auth.Principal, id, content string, emit func(string) error) (Message, provider.Usage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Message{}, provider.Usage{}, fmt.Errorf("消息不能为空")
	}
	var c Conversation
	if err := s.DB.QueryRow(ctx, `SELECT id,workspace_id,user_id,agent_id,title,created_at,updated_at FROM conversations WHERE id=$1 AND workspace_id=$2 AND user_id=$3`, id, p.WorkspaceID, p.UserID).Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return Message{}, provider.Usage{}, fmt.Errorf("会话不存在")
		}
		return Message{}, provider.Usage{}, err
	}
	now := time.Now().UTC()
	userID := "msg_" + uuid.NewString()
	if _, err := s.DB.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status,created_at) VALUES($1,$2,'user',$3,'completed',$4)`, userID, id, content, now); err != nil {
		return Message{}, provider.Usage{}, err
	}
	var version definition.AgentVersion
	a, err := s.Agents.Get(ctx, p, c.AgentID)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	version, err = s.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	history, err := s.Messages(ctx, p, id)
	if err != nil {
		return Message{}, provider.Usage{}, err
	}
	msgs := []provider.Message{}
	for _, slot := range version.Configuration.Prompts {
		if slot.Enabled && strings.TrimSpace(slot.Content) != "" {
			msgs = append(msgs, provider.Message{Role: "system", Content: slot.Content})
		}
	}
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}
	for _, m := range history[start:] {
		msgs = append(msgs, provider.Message{Role: m.Role, Content: m.Content})
	}
	var answer strings.Builder
	began := time.Now()
	usage, callErr := s.Model.Stream(ctx, s.ModelName, msgs, func(d string) error { answer.WriteString(d); return emit(d) })
	latency := time.Since(began).Milliseconds()
	status := "completed"
	errText := ""
	if callErr != nil {
		status = "failed"
		errText = callErr.Error()
	}
	assistantID := "msg_" + uuid.NewString()
	_, dbErr := s.DB.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,model,prompt_tokens,completion_tokens,latency_ms,status,error,created_at) VALUES($1,$2,'assistant',$3,$4,$5,$6,$7,$8,$9,$10)`, assistantID, id, answer.String(), s.ModelName, usage.PromptTokens, usage.CompletionTokens, latency, status, errText, time.Now().UTC())
	if dbErr != nil {
		return Message{}, usage, dbErr
	}
	if c.Title == "" {
		r := []rune(content)
		if len(r) > 30 {
			r = r[:30]
		}
		_, dbErr = s.DB.Exec(ctx, `UPDATE conversations SET title=$1,updated_at=now() WHERE id=$2`, string(r), id)
	} else {
		_, dbErr = s.DB.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, id)
	}
	if dbErr != nil {
		return Message{}, usage, dbErr
	}
	return Message{ID: assistantID, ConversationID: id, Role: "assistant", Content: answer.String(), Model: s.ModelName, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, LatencyMS: latency, Status: status, Error: errText}, usage, callErr
}
