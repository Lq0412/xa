package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

// PGStore 实现 PostgreSQL 版 Store。
type PGStore struct{ DB *pgxpool.Pool }

func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{DB: db} }

func (s *PGStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.DB.Begin(ctx)
}

func (s *PGStore) InsertRun(ctx context.Context, tx pgx.Tx, run Run) error {
	limits, _ := json.Marshal(run.Limits)
	toolset, _ := json.Marshal(run.Toolset)
	_, err := tx.Exec(ctx, `
INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,status,user_message_id,provider,model,stream_protocol,limits,toolset,toolset_hash,started_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		run.ID, run.WorkspaceID, run.OwnerUserID, run.ConversationID, run.AgentID, run.AgentVersionID,
		run.RootRunID, run.Status, run.UserMessageID, run.Provider, run.Model, run.StreamProtocol,
		limits, toolset, run.Toolset.Hash, run.StartedAt)
	return err
}

func (s *PGStore) UpdateRun(ctx context.Context, tx pgx.Tx, run Run) error {
	_, err := tx.Exec(ctx, `
UPDATE runs SET status=$2,steps=$3,model_turns=$4,tool_calls=$5,prompt_tokens=$6,completion_tokens=$7,
cached_tokens=$8,latency_ms=$9,active_ms=$10,error_code=$11,error=$12,final_message_id=$13,ended_at=$14,updated_at=now()
WHERE id=$1`,
		run.ID, run.Status, run.Steps, run.ModelTurns, run.ToolCalls, run.PromptTokens, run.CompletionTokens,
		run.CachedTokens, run.LatencyMS, run.ActiveMS, run.ErrorCode, run.Error, run.FinalMessageID, run.EndedAt)
	return err
}

func (s *PGStore) GetRun(ctx context.Context, id string) (Run, error) {
	var run Run
	var limitsRaw, toolsetRaw []byte
	var ended *time.Time
	err := s.DB.QueryRow(ctx, `
SELECT id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,status,user_message_id,
final_message_id,provider,model,stream_protocol,limits,toolset,steps,model_turns,tool_calls,prompt_tokens,completion_tokens,
cached_tokens,latency_ms,active_ms,error_code,error,started_at,ended_at
FROM runs WHERE id=$1`, id).Scan(
		&run.ID, &run.WorkspaceID, &run.OwnerUserID, &run.ConversationID, &run.AgentID, &run.AgentVersionID,
		&run.RootRunID, &run.Status, &run.UserMessageID, &run.FinalMessageID, &run.Provider, &run.Model, &run.StreamProtocol,
		&limitsRaw, &toolsetRaw, &run.Steps, &run.ModelTurns, &run.ToolCalls, &run.PromptTokens, &run.CompletionTokens,
		&run.CachedTokens, &run.LatencyMS, &run.ActiveMS, &run.ErrorCode, &run.Error, &run.StartedAt, &ended,
	)
	if err != nil {
		return Run{}, err
	}
	run.EndedAt = ended
	_ = json.Unmarshal(limitsRaw, &run.Limits)
	var ts tool.Toolset
	_ = json.Unmarshal(toolsetRaw, &ts)
	run.Toolset = ts
	return run, nil
}

func (s *PGStore) NextOrdinal(ctx context.Context, tx pgx.Tx, runID string) (int, error) {
	var n int
	q := `SELECT COALESCE(MAX(ordinal),0)+1 FROM run_messages WHERE run_id=$1`
	if tx != nil {
		err := tx.QueryRow(ctx, q, runID).Scan(&n)
		return n, err
	}
	err := s.DB.QueryRow(ctx, q, runID).Scan(&n)
	return n, err
}

func (s *PGStore) AppendRunMessage(ctx context.Context, tx pgx.Tx, msg RunMessage) error {
	if msg.ID == "" {
		msg.ID = "rm_" + uuid.NewString()
	}
	if msg.Ordinal == 0 {
		n, err := s.NextOrdinal(ctx, tx, msg.RunID)
		if err != nil {
			return err
		}
		msg.Ordinal = n
	}
	var toolCallsParam any
	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		raw, _ := json.Marshal(msg.ToolCalls)
		toolCallsParam = raw
	}
	var toolCallID, toolName any
	if msg.Role == "tool" {
		if strings.TrimSpace(msg.ToolCallID) == "" {
			return fmt.Errorf("tool 消息缺少 tool_call_id")
		}
		toolCallID = msg.ToolCallID
		toolName = nullable(msg.ToolName)
	}
	exec := func(ctx context.Context, q pgx.Tx) error {
		_, err := q.Exec(ctx, `
INSERT INTO run_messages(id,run_id,step,ordinal,role,content,tool_calls,tool_call_id,tool_name,finish_reason)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			msg.ID, msg.RunID, msg.Step, msg.Ordinal, msg.Role, msg.Content, toolCallsParam,
			toolCallID, toolName, nullable(msg.FinishReason))
		return err
	}
	if tx != nil {
		return exec(ctx, tx)
	}
	t, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = t.Rollback(ctx) }()
	if err := exec(ctx, t); err != nil {
		return err
	}
	return t.Commit(ctx)
}

func (s *PGStore) ListRunMessages(ctx context.Context, runID string) ([]RunMessage, error) {
	rows, err := s.DB.Query(ctx, `
SELECT id,run_id,step,ordinal,role,content,tool_calls,tool_call_id,tool_name,finish_reason
FROM run_messages WHERE run_id=$1 ORDER BY ordinal`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunMessage{}
	for rows.Next() {
		var m RunMessage
		var toolCalls []byte
		var tcID, tname, fr *string
		if err := rows.Scan(&m.ID, &m.RunID, &m.Step, &m.Ordinal, &m.Role, &m.Content, &toolCalls, &tcID, &tname, &fr); err != nil {
			return nil, err
		}
		if len(toolCalls) > 0 {
			_ = json.Unmarshal(toolCalls, &m.ToolCalls)
		}
		if tcID != nil {
			m.ToolCallID = *tcID
		}
		if tname != nil {
			m.ToolName = *tname
		}
		if fr != nil {
			m.FinishReason = *fr
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PGStore) InsertToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error {
	if row.ID == "" {
		row.ID = NewResultRef()
	}
	_, err := tx.Exec(ctx, `
INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,tool_version,effect,
arguments,raw_arguments,args_hash,status,error_code,error_message,model_content,data,display,truncated,total_chars,attempts,idempotency_key)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		row.ID, row.RunID, row.ConversationID, row.OwnerUserID, row.Step, row.CallIndex, row.CallID, row.ToolName,
		row.ToolVersion, row.Effect, row.Arguments, row.RawArguments, row.ArgsHash, row.Status, row.ErrorCode,
		row.ErrorMessage, row.ModelContent, row.Data, row.Display, row.Truncated, row.TotalChars, row.Attempts,
		row.CallID)
	return err
}

func (s *PGStore) UpdateToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error {
	_, err := tx.Exec(ctx, `
UPDATE tool_results SET status=$2,error_code=$3,error_message=$4,model_content=$5,data=$6,display=$7,
truncated=$8,total_chars=$9,attempts=$10,ended_at=now() WHERE id=$1`,
		row.ID, row.Status, row.ErrorCode, row.ErrorMessage, row.ModelContent, row.Data, row.Display,
		row.Truncated, row.TotalChars, row.Attempts)
	return err
}

func (s *PGStore) GetToolResult(ctx context.Context, id string) (ToolResultRow, error) {
	var row ToolResultRow
	err := s.DB.QueryRow(ctx, `SELECT id,run_id,conversation_id,owner_user_id,step,call_id,tool_name,status,model_content,data,truncated,total_chars FROM tool_results WHERE id=$1`, id).
		Scan(&row.ID, &row.RunID, &row.ConversationID, &row.OwnerUserID, &row.Step, &row.CallID, &row.ToolName, &row.Status, &row.ModelContent, &row.Data, &row.Truncated, &row.TotalChars)
	return row, err
}

func (s *PGStore) GetToolResultByCallID(ctx context.Context, runID, callID string) (ToolResultRow, error) {
	var row ToolResultRow
	err := s.DB.QueryRow(ctx, `SELECT id,run_id,call_id,tool_name,status,model_content FROM tool_results WHERE run_id=$1 AND call_id=$2`, runID, callID).
		Scan(&row.ID, &row.RunID, &row.CallID, &row.ToolName, &row.Status, &row.ModelContent)
	return row, err
}

func (s *PGStore) SaveRunContext(ctx context.Context, tx pgx.Tx, snap RunContextSnapshot) error {
	msgs, _ := json.Marshal(snap.Messages)
	_, err := tx.Exec(ctx, `INSERT INTO run_contexts(run_id,messages,segments) VALUES($1,$2,$3)
ON CONFLICT (run_id) DO UPDATE SET messages=EXCLUDED.messages, segments=EXCLUDED.segments`,
		snap.RunID, msgs, snap.Segments)
	return err
}

func (s *PGStore) LoadRunContext(ctx context.Context, runID string) (RunContextSnapshot, error) {
	var snap RunContextSnapshot
	var msgs []byte
	snap.RunID = runID
	err := s.DB.QueryRow(ctx, `SELECT messages,segments FROM run_contexts WHERE run_id=$1`, runID).Scan(&msgs, &snap.Segments)
	if err != nil {
		return snap, err
	}
	_ = json.Unmarshal(msgs, &snap.Messages)
	return snap, nil
}

func (s *PGStore) ListRecentCompletedToolRuns(ctx context.Context, conversationID string, limit int) ([]Run, error) {
	rows, err := s.DB.Query(ctx, `
SELECT id,conversation_id,status,tool_calls,started_at FROM runs
WHERE conversation_id=$1 AND status='completed' AND tool_calls>0
ORDER BY started_at DESC LIMIT $2`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.Status, &r.ToolCalls, &r.StartedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) GetMessageRunIDs(ctx context.Context, messageIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(messageIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT id,run_id FROM messages WHERE id = ANY($1) AND run_id IS NOT NULL`, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rid string
		if err := rows.Scan(&id, &rid); err != nil {
			return nil, err
		}
		out[id] = rid
	}
	return out, rows.Err()
}

func (s *PGStore) PendingCallsOfLastStep(ctx context.Context, runID string) ([]PlannedCall, error) {
	return nil, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ParseLimitsFromVersion 从 Agent 版本解析 runtime limits。
func ParseLimitsFromVersion(v definition.AgentVersion) (definition.RuntimeLimits, error) {
	return definition.ParseRuntimeLimits(v.Configuration.RuntimeLimits)
}

// MessagesToProvider 将快照消息反序列化。
func MessagesToProvider(raw []provider.Message) []provider.Message { return raw }
