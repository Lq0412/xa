package run

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"sort"
)

// SpanSink 隔离追踪采集与存储，后续 exporter 可复用同一批快照。
type SpanSink interface {
	WriteSpans(context.Context, []Span) error
}

// PGSpanSink 使用调用方事务，或在独立短事务中写入。
type PGSpanSink struct {
	Store Store
	Tx    pgx.Tx
}

func (s PGSpanSink) WriteSpans(ctx context.Context, spans []Span) error {
	if s.Tx != nil {
		return s.Store.UpsertSpans(ctx, s.Tx, spans)
	}
	tx, err := s.Store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.Store.UpsertSpans(ctx, tx, spans); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpsertSpans 在同一事务内保存节点；重复写入只更新同一个节点。
func (s *PGStore) UpsertSpans(ctx context.Context, tx pgx.Tx, spans []Span) error {
	for _, s := range spans {
		s = RedactSpan(s)
		attrs, err := json.Marshal(s.Attributes)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO run_spans(id,run_id,parent_span_id,kind,name,step,call_id,status,error_code,attributes,input_preview,output_preview,prompt_tokens,completion_tokens,cached_tokens,estimated_tokens,started_at,ended_at,duration_ms) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,error_code=EXCLUDED.error_code,attributes=EXCLUDED.attributes,input_preview=EXCLUDED.input_preview,output_preview=EXCLUDED.output_preview,prompt_tokens=EXCLUDED.prompt_tokens,completion_tokens=EXCLUDED.completion_tokens,cached_tokens=EXCLUDED.cached_tokens,estimated_tokens=EXCLUDED.estimated_tokens,ended_at=EXCLUDED.ended_at,duration_ms=EXCLUDED.duration_ms`, s.ID, s.RunID, nullable(s.ParentSpanID), s.Kind, s.Name, s.Step, nullable(s.CallID), s.Status, s.ErrorCode, attrs, s.InputPreview, s.OutputPreview, s.PromptTokens, s.CompletionTokens, s.CachedTokens, s.EstimatedTokens, s.StartedAt, s.EndedAt, s.DurationMS)
		if err != nil {
			return err
		}
	}
	return nil
}

// ListSpans 按执行时间返回节点，旧 run 返回空列表。
func (s *PGStore) ListSpans(ctx context.Context, runID string) ([]Span, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,run_id,COALESCE(parent_span_id,''),kind,name,COALESCE(step,0),COALESCE(call_id,''),status,error_code,attributes,input_preview,output_preview,prompt_tokens,completion_tokens,cached_tokens,estimated_tokens,started_at,ended_at,duration_ms FROM run_spans WHERE run_id=$1 ORDER BY started_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Span{}
	for rows.Next() {
		var s Span
		var attrs []byte
		if err := rows.Scan(&s.ID, &s.RunID, &s.ParentSpanID, &s.Kind, &s.Name, &s.Step, &s.CallID, &s.Status, &s.ErrorCode, &attrs, &s.InputPreview, &s.OutputPreview, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.EstimatedTokens, &s.StartedAt, &s.EndedAt, &s.DurationMS); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(attrs, &s.Attributes); err != nil {
			return nil, err
		}
		out = append(out, RedactSpan(s))
	}
	return out, rows.Err()
}

// UpsertSpans 的内存实现供引擎测试使用。
func (m *MemStore) UpsertSpans(ctx context.Context, tx pgx.Tx, spans []Span) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.spans == nil {
		m.spans = map[string]Span{}
	}
	for _, s := range spans {
		m.spans[s.ID] = RedactSpan(s)
	}
	return nil
}
func (m *MemStore) ListSpans(ctx context.Context, runID string) ([]Span, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Span{}
	for _, s := range m.spans {
		if s.RunID == runID {
			out = append(out, RedactSpan(s))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}
