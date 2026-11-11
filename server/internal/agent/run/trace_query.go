package run

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/redact"
)

// RunFilter 使用稳定的复合游标，避免相同创建时间导致重复或漏行。
type RunFilter struct {
	ConversationID, AgentID, Status, UserID, Cursor string
	Limit                                           int
}

type runCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// RunPage 保留旧调用方解码列表时使用的 map 视图。
type RunPage struct {
	Runs       []map[string]any `json:"runs"`
	NextCursor string           `json:"next_cursor"`
}

// RunList 返回固定的运行摘要与下一页位置，空列表编码为数组。
type RunList struct {
	Runs       []RunSummary `json:"runs" nullable:"false"`
	NextCursor string       `json:"next_cursor"`
}

// RunSummary 是运行列表与详情共用的脱敏字段。
type RunSummary struct {
	ID               string  `json:"id"`
	ConversationID   string  `json:"conversation_id"`
	AgentID          string  `json:"agent_id"`
	AgentName        string  `json:"agent_name"`
	UserEmail        *string `json:"user_email,omitempty"`
	Status           string  `json:"status"`
	Model            string  `json:"model"`
	Steps            int     `json:"steps"`
	ToolCalls        int     `json:"tool_calls"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	TTFTMS           *int    `json:"ttft_ms"`
	LatencyMS        int64   `json:"latency_ms"`
	ActiveMS         int64   `json:"active_ms"`
	ErrorCode        string  `json:"error_code"`
	CreatedAt        string  `json:"created_at" format:"date-time"`
	StartedAt        *string `json:"started_at" format:"date-time"`
	EndedAt          *string `json:"ended_at" format:"date-time"`
}

// RunDetail 补充本次运行使用的配置与消息引用。
type RunDetail struct {
	RunSummary
	WorkspaceID    string          `json:"workspace_id"`
	OwnerUserID    string          `json:"owner_user_id"`
	AgentVersionID string          `json:"agent_version_id"`
	UserMessageID  string          `json:"user_message_id"`
	FinalMessageID string          `json:"final_message_id"`
	Provider       string          `json:"provider"`
	StreamProtocol int             `json:"stream_protocol"`
	Limits         json.RawMessage `json:"limits"`
	ToolsetHash    string          `json:"toolset_hash"`
	Toolset        []*string       `json:"toolset" nullable:"false"`
	Error          string          `json:"error"`
	CancelReason   string          `json:"cancel_reason"`
}

// ToolResultDetail 保留工具产生的动态 JSON 载荷。
type ToolResultDetail struct {
	RunID        string          `json:"run_id"`
	CallID       string          `json:"call_id"`
	ToolName     string          `json:"tool_name"`
	Status       string          `json:"status"`
	Arguments    json.RawMessage `json:"arguments"`
	Display      json.RawMessage `json:"display"`
	ModelContent string          `json:"model_content"`
	Data         json.RawMessage `json:"data"`
}

func validateRunFilter(f RunFilter) (RunFilter, *runCursor, error) {
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || (f.Status != "" && !slices.Contains([]string{"queued", "running", "waiting_approval", "completed", "failed", "cancelled", "budget_exhausted", "interrupted"}, f.Status)) {
		return f, nil, definition.ErrInvalid
	}
	if f.Cursor == "" {
		return f, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
	if err != nil {
		return f, nil, definition.ErrInvalid
	}
	var cursor runCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
		return f, nil, definition.ErrInvalid
	}
	return f, &cursor, nil
}

// 固定白名单投影不带 lease、幂等键、原始上下文或工具载荷。
const runSummaryJSON = `jsonb_build_object('id',r.id,'conversation_id',r.conversation_id,'agent_id',r.agent_id,'agent_name',d.name,'status',r.status,'model',r.model,'steps',r.steps,'tool_calls',r.tool_calls,'prompt_tokens',r.prompt_tokens,'completion_tokens',r.completion_tokens,'cached_tokens',r.cached_tokens,'ttft_ms',r.ttft_ms,'latency_ms',r.latency_ms,'active_ms',r.active_ms,'error_code',r.error_code,'created_at',r.created_at,'started_at',r.started_at,'ended_at',r.ended_at)`

func requireRunScope(p auth.Principal, admin bool) error {
	if p.UserID == "" || p.WorkspaceID == "" {
		return definition.ErrNotFound
	}
	if admin && !auth.HasRole(p, "admin") {
		return definition.ErrForbidden
	}
	return nil
}

// ListRuns 对 owner 和管理员共享分页实现，但权限条件始终在 SQL 内生效。
func (s *RunService) ListRuns(ctx context.Context, p auth.Principal, f RunFilter, admin bool) (RunList, error) {
	if err := requireRunScope(p, admin); err != nil {
		return RunList{}, err
	}
	f, cursor, err := validateRunFilter(f)
	if err != nil || (!admin && f.UserID != "") {
		return RunList{}, definition.ErrInvalid
	}
	var cursorAt *time.Time
	var cursorID string
	if cursor != nil {
		cursorAt = &cursor.CreatedAt
		cursorID = cursor.ID
	}
	projection := runSummaryJSON
	if admin {
		projection += ` || jsonb_build_object('user_email',u.email)`
	}
	rows, err := s.DB.Query(ctx, `SELECT `+projection+`,r.created_at,r.id FROM runs r JOIN agent_definitions d ON d.id=r.agent_id JOIN users u ON u.id=r.owner_user_id
WHERE r.workspace_id=$1 AND ($2 OR r.owner_user_id=$3) AND ($4='' OR r.conversation_id=$4) AND ($5='' OR r.agent_id=$5) AND ($6='' OR r.status=$6) AND ($7='' OR r.owner_user_id=$7) AND ($8::timestamptz IS NULL OR (r.created_at,r.id)<($8,$9)) ORDER BY r.created_at DESC,r.id DESC LIMIT $10`, p.WorkspaceID, admin, p.UserID, f.ConversationID, f.AgentID, f.Status, f.UserID, cursorAt, cursorID, f.Limit+1)
	if err != nil {
		return RunList{}, err
	}
	defer rows.Close()
	page := RunList{Runs: []RunSummary{}}
	var last runCursor
	for rows.Next() {
		var raw []byte
		var position runCursor
		if err := rows.Scan(&raw, &position.CreatedAt, &position.ID); err != nil {
			return RunList{}, err
		}
		if len(page.Runs) == f.Limit {
			encoded, err := json.Marshal(last)
			if err != nil {
				return RunList{}, err
			}
			page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			break
		}
		var value RunSummary
		if err := json.Unmarshal(raw, &value); err != nil {
			return RunList{}, err
		}
		if err := redactRunRecord(&value); err != nil {
			return RunList{}, err
		}
		page.Runs = append(page.Runs, value)
		last = position
	}
	return page, rows.Err()
}

func redactRunRecord(value *RunSummary) error {
	value.AgentName = redact.String(value.AgentName)
	value.ErrorCode = redact.String(value.ErrorCode)
	// 查询时间和 SSE 使用同一时区；数据库依然保存 timestamptz。
	for _, field := range []struct {
		name string
		text *string
	}{{"created_at", &value.CreatedAt}, {"started_at", value.StartedAt}, {"ended_at", value.EndedAt}} {
		if field.text == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, *field.text)
		if err != nil {
			return fmt.Errorf("解析运行时间 %s: %w", field.name, err)
		}
		*field.text = at.In(shanghaiLoc).Format(time.RFC3339Nano)
	}
	return nil
}

// QueryRun 只返回允许出站的详情字段，跨 owner 或 workspace 均表现为不存在。
func (s *RunService) QueryRun(ctx context.Context, p auth.Principal, id string, admin bool) (RunDetail, error) {
	if err := requireRunScope(p, admin); err != nil {
		return RunDetail{}, err
	}
	projection := runSummaryJSON + ` || jsonb_build_object('workspace_id',r.workspace_id,'owner_user_id',r.owner_user_id,'agent_version_id',r.agent_version_id,'user_message_id',r.user_message_id,'final_message_id',coalesce(r.final_message_id,''),'provider',r.provider,'stream_protocol',r.stream_protocol,'limits',r.limits,'toolset_hash',r.toolset_hash,'toolset',coalesce((SELECT jsonb_agg(coalesce(entry->'spec'->>'Name',entry->'spec'->>'name')) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(r.toolset)='object' THEN coalesce(nullif(r.toolset->'entries','null'::jsonb),'[]'::jsonb) ELSE r.toolset END) entry),'[]'::jsonb),'error',r.error,'cancel_reason',r.cancel_reason)`
	if admin {
		projection += ` || jsonb_build_object('user_email',u.email)`
	}
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT `+projection+` FROM runs r JOIN agent_definitions d ON d.id=r.agent_id JOIN users u ON u.id=r.owner_user_id WHERE r.id=$1 AND r.workspace_id=$2 AND ($3 OR r.owner_user_id=$4)`, id, p.WorkspaceID, admin, p.UserID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return RunDetail{}, definition.ErrNotFound
	}
	if err != nil {
		return RunDetail{}, err
	}
	var value RunDetail
	if err := json.Unmarshal(raw, &value); err != nil {
		return RunDetail{}, err
	}
	if err := redactRunRecord(&value.RunSummary); err != nil {
		return RunDetail{}, err
	}
	value.Error = redact.String(value.Error)
	return value, nil
}

// QueryToolResult 只有 owner 可读取原始 data，管理员没有对应的放宽入口。
func (s *RunService) QueryToolResult(ctx context.Context, p auth.Principal, runID, callID string) (ToolResultDetail, error) {
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT jsonb_build_object('run_id',t.run_id,'call_id',t.call_id,'tool_name',t.tool_name,'status',t.status,'arguments',t.arguments,'display',t.display,'model_content',t.model_content,'data',t.data) FROM tool_results t JOIN runs r ON r.id=t.run_id WHERE t.run_id=$1 AND t.call_id=$2 AND r.workspace_id=$3 AND r.owner_user_id=$4`, runID, callID, p.WorkspaceID, p.UserID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return ToolResultDetail{}, definition.ErrNotFound
	}
	if err != nil {
		return ToolResultDetail{}, err
	}
	var value ToolResultDetail
	if err := json.Unmarshal(raw, &value); err != nil {
		return ToolResultDetail{}, err
	}
	return value, nil
}

// SpanNode 为 API 显式保留 nullable step 及空 children 数组。
type SpanNode struct {
	Span
	Step     *int        `json:"step"`
	CallID   string      `json:"call_id"`
	Children []*SpanNode `json:"children" nullable:"false"`
}
type SpanTree struct {
	RunID string    `json:"run_id"`
	Span  *SpanNode `json:"span"`
}

// BuildSpanTree 修复查询视图内的孤儿与环，不修改持久化数据。
func BuildSpanTree(runID string, spans []Span) SpanTree {
	result := SpanTree{RunID: runID}
	ordered := append([]Span(nil), spans...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartedAt.Equal(ordered[j].StartedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].StartedAt.Before(ordered[j].StartedAt)
	})
	nodes := map[string]*SpanNode{}
	for _, span := range ordered {
		node := &SpanNode{Span: RedactSpan(span), CallID: span.CallID, Children: []*SpanNode{}}
		node.StartedAt = node.StartedAt.In(shanghaiLoc)
		if node.EndedAt != nil {
			ended := node.EndedAt.In(shanghaiLoc)
			node.EndedAt = &ended
		}
		if span.Step > 0 {
			step := span.Step
			node.Step = &step
		}
		nodes[span.ID] = node
		if span.Kind == "run" && result.Span == nil {
			result.Span = node
		}
	}
	if len(ordered) == 0 {
		return result
	}
	if result.Span == nil {
		result.Span = nodes[ordered[0].ID]
	}
	anomaly := false
	for _, span := range ordered {
		node := nodes[span.ID]
		if node == result.Span {
			continue
		}
		parent := nodes[span.ParentSpanID]
		seen := map[string]bool{span.ID: true}
		candidate := parent
		for candidate != nil {
			if seen[candidate.ID] {
				parent = nil
				break
			}
			seen[candidate.ID] = true
			candidate = nodes[candidate.ParentSpanID]
		}
		if parent == nil {
			parent = result.Span
			anomaly = true
		}
		parent.Children = append(parent.Children, node)
	}
	if anomaly {
		result.Span.Attributes["anomalies"] = []string{"orphaned_span"}
		slog.Warn("运行追踪存在孤儿或环", "run_id", runID)
	}
	return result
}

// QuerySpans 先判定 run 权限，再读取树，旧 run 返回 span:null。
func (s *RunService) QuerySpans(ctx context.Context, p auth.Principal, id string, admin bool) (SpanTree, error) {
	if _, err := s.QueryRun(ctx, p, id, admin); err != nil {
		return SpanTree{}, err
	}
	spans, err := s.Store.ListSpans(ctx, id)
	if err != nil {
		return SpanTree{}, fmt.Errorf("读取执行树: %w", err)
	}
	return BuildSpanTree(id, spans), nil
}
