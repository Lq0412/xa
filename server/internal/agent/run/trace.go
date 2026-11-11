package run

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/framework/redact"
	"linq-bs/server/internal/tool"
)

// Span 是一次操作的脱敏追踪快照；完整上下文与工具载荷不进入此结构。
type Span struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	ParentSpanID     string         `json:"parent_span_id,omitempty"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	Step             int            `json:"step"`
	CallID           string         `json:"call_id,omitempty"`
	Status           string         `json:"status"`
	ErrorCode        string         `json:"error_code"`
	Attributes       map[string]any `json:"attributes"`
	InputPreview     string         `json:"input_preview"`
	OutputPreview    string         `json:"output_preview"`
	PromptTokens     *int           `json:"prompt_tokens"`
	CompletionTokens *int           `json:"completion_tokens"`
	CachedTokens     *int           `json:"cached_tokens"`
	EstimatedTokens  *int           `json:"estimated_tokens"`
	StartedAt        time.Time      `json:"started_at"`
	EndedAt          *time.Time     `json:"ended_at"`
	DurationMS       *int64         `json:"duration_ms"`
}

// TracePreview 在截断前脱敏，省略号计入长度上限。
func TracePreview(s string) string {
	r := []rune(redact.String(s))
	if len(r) > 512 {
		return string(r[:511]) + "…"
	}
	return string(r)
}

// RedactSpan 在写入和出站时递归脱敏，防止嵌套属性带出凭证。
func RedactSpan(s Span) Span {
	s.Name = TracePreview(s.Name)
	s.ErrorCode = TracePreview(s.ErrorCode)
	s.InputPreview = TracePreview(s.InputPreview)
	s.OutputPreview = TracePreview(s.OutputPreview)
	raw, err := json.Marshal(s.Attributes)
	if err != nil {
		panic(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(raw, &attrs); err != nil {
		panic(err)
	}
	var clean func(any) any
	clean = func(v any) any {
		switch x := v.(type) {
		case string:
			return TracePreview(x)
		case map[string]any:
			out := map[string]any{}
			for k, v := range x {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "secret") || strings.Contains(lk, "password") || strings.Contains(lk, "api_key") || strings.Contains(lk, "authorization") {
					out[TracePreview(k)] = "[REDACTED]"
				} else {
					out[TracePreview(k)] = clean(v)
				}
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, v := range x {
				out[i] = clean(v)
			}
			return out
		default:
			return v
		}
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	s.Attributes = clean(attrs).(map[string]any)
	return s
}

// Tracer 串行保护内存树；恢复时复用已持久化节点的 ID。
type Tracer struct {
	mu            sync.Mutex
	flushMu       sync.Mutex
	RunID, RootID string
	spans         map[string]Span
	Sink          SpanSink
}

// NewTracer 创建或恢复同一轮的追踪。
func NewTracer(run Run, sink SpanSink, existing []Span) *Tracer {
	t := &Tracer{RunID: run.ID, Sink: sink, spans: map[string]Span{}}
	for _, s := range existing {
		t.spans[s.ID] = s
		if s.Kind == "run" {
			t.RootID = s.ID
		}
	}
	if t.RootID == "" {
		s := t.Start("", "run", "invoke_agent "+run.AgentID, 0, "", run.StartedAt)
		t.RootID = s.ID
		s.Attributes = map[string]any{"agent_id": run.AgentID, "agent_version_id": run.AgentVersionID, "model": run.Model, "provider": run.Provider, "toolset_hash": run.Toolset.Hash, "stream_protocol": run.StreamProtocol}
		t.Put(s)
	}
	return t
}

// Start 生成子节点；空 parent 表示根节点。
func (t *Tracer) Start(parent, kind, name string, step int, callID string, at time.Time) Span {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	s := Span{ID: "spn_" + uuid.NewString(), RunID: t.RunID, ParentSpanID: parent, Kind: kind, Name: name, Step: step, CallID: callID, Status: "ok", StartedAt: at, Attributes: map[string]any{}}
	t.Put(s)
	return s
}

// Put 更新节点内容，保留调用方明确指定的开始时间与 ID。
func (t *Tracer) Put(s Span) { t.mu.Lock(); defer t.mu.Unlock(); t.spans[s.ID] = RedactSpan(s) }

// End 结束节点并计算实际耗时。
func (t *Tracer) End(s Span, status, code string, at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	s.Status = status
	s.ErrorCode = code
	s.EndedAt = &at
	ms := at.Sub(s.StartedAt).Milliseconds()
	s.DurationMS = &ms
	t.Put(s)
}

// Spans 返回与内部状态隔离的快照。
func (t *Tracer) Spans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Span, 0, len(t.spans))
	for _, s := range t.spans {
		out = append(out, RedactSpan(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FlushStep 将当前树写入 sink，调用方控制事务边界。
func (t *Tracer) FlushStep(ctx context.Context) error {
	t.flushMu.Lock()
	defer t.flushMu.Unlock()
	return t.Sink.WriteSpans(ctx, t.Spans())
}

// Finish 关闭未结束节点，根 token 始终使用 run 的累计值。
func (t *Tracer) Finish(run Run) {
	status := "ok"
	if run.Status == "cancelled" {
		status = "cancelled"
	} else if run.Status != "completed" {
		status = "error"
	}
	at := time.Now().UTC()
	if run.EndedAt != nil {
		at = *run.EndedAt
	}
	for _, s := range t.Spans() {
		if s.ID == t.RootID {
			s.PromptTokens = &run.PromptTokens
			s.CompletionTokens = &run.CompletionTokens
			s.CachedTokens = &run.CachedTokens
		}
		if s.EndedAt == nil {
			t.End(s, status, run.ErrorCode, at)
		} else {
			t.Put(s)
		}
	}
}

// initTrace 将准备期间采集的耗时挂到新 run 下。
func (rc *RunContext) initTrace(in ChatStartInput) {
	r := rc.Run
	if !in.ContextStartedAt.IsZero() && in.ContextStartedAt.Before(r.StartedAt) {
		r.StartedAt = in.ContextStartedAt
	}
	rc.Tracer = NewTracer(r, PGSpanSink{Store: rc.Store, Tx: rc.Tx}, nil)
	t := rc.Tracer
	c := t.Start(t.RootID, "context", "context", 0, "", in.ContextStartedAt)
	for _, o := range in.ContextTrace {
		kind := "context_segment"
		if o.Segment.Name == "memory_recall" {
			kind = "memory_recall"
		}
		s := t.Start(c.ID, kind, o.Segment.Name, 0, "", o.StartedAt)
		s.EstimatedTokens = &o.Segment.EstimatedTokens
		s.Attributes = map[string]any{"slot": o.Slot, "item_ids": o.Segment.ItemIDs}
		for k, v := range o.Segment.Attributes {
			s.Attributes[k] = v
		}
		t.End(s, "ok", "", o.EndedAt)
	}
	t.End(c, "ok", "", in.ContextEndedAt)
}

func (rc *RunContext) recordTool(row ToolResultRow, started time.Time, effects ...tool.Effect2) error {
	if rc.Tracer == nil {
		return nil
	}
	t := rc.Tracer
	s := t.Start(t.RootID, "tool", "execute_tool "+row.ToolName, row.Step, row.CallID, started)
	s.InputPreview = TracePreview(row.RawArguments)
	s.OutputPreview, _ = toolEventSummary(row)
	s.Attributes = map[string]any{"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": row.ToolName, "tool_version": row.ToolVersion, "result_ref": row.ID, "effect": row.Effect, "truncated": row.Truncated, "attempts": row.Attempts}
	status := "ok"
	if row.Status == "cancelled" {
		status = "cancelled"
	} else if row.Status != "succeeded" {
		status = "error"
	}
	ended := time.Now().UTC()
	t.End(s, status, row.ErrorCode, ended)
	if row.Status == "succeeded" {
		if row.ToolName == "memory_search" {
			child := t.Start(s.ID, "memory_recall", "memory_recall", row.Step, row.CallID, started)
			child.OutputPreview = s.OutputPreview
			t.End(child, "ok", "", ended)
		}
		for _, effect := range effects {
			if strings.HasPrefix(effect.Kind, "memory.") {
				child := t.Start(s.ID, "memory_write", "memory_write", row.Step, row.CallID, started)
				child.Attributes = map[string]any{"op": strings.TrimPrefix(effect.Kind, "memory."), "memory_id": effect.TargetID}
				child.OutputPreview = s.OutputPreview
				t.End(child, "ok", "", ended)
			}
		}
	}
	return t.FlushStep(context.Background())
}

func (t *Tracer) closeApproval(callID, decision, remember string, at time.Time) {
	for _, s := range t.Spans() {
		if s.Kind == "approval" && s.CallID == callID && s.EndedAt == nil {
			s.Attributes["decision"] = decision
			if remember != "" {
				s.Attributes["remember"] = remember
			}
			s.Attributes["wait_ms"] = at.Sub(s.StartedAt).Milliseconds()
			status := "ok"
			if decision == "cancelled" {
				status = "cancelled"
			}
			t.End(s, status, "", at)
		}
	}
}
