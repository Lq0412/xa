package run

import (
	"context"
	"fmt"
	"time"
)

const terminalRunSQL = `runs.status IN ('completed','failed','cancelled','budget_exhausted','interrupted') AND runs.ended_at IS NOT NULL`

// CleanupRetention 分批清理终态运行的到期载荷；事件与过程快照在同一事务处理。
// 调用方持有 sweeper 顾问锁，此处的两秒预算只限制本轮工作量，不中断正在提交的批次。
func (s *RunService) CleanupRetention(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("保留期清理需要 PostgreSQL")
	}
	events, contexts, traces := s.Config.EventRetention, s.Config.ContextRetention, s.Config.TraceRetention
	if events <= 0 {
		events = 168 * time.Hour
	}
	if contexts <= 0 {
		contexts = 168 * time.Hour
	}
	if traces <= 0 {
		traces = 720 * time.Hour
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count, err := s.cleanupRetentionBatch(ctx, time.Now().Add(-events), time.Now().Add(-contexts), time.Now().Add(-traces))
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
	}
	return nil
}

func (s *RunService) cleanupRetentionBatch(ctx context.Context, eventCutoff, contextCutoff, traceCutoff time.Time) (int64, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	// 锁住终态 run，防止并行清理器对同一轮重复补快照；每批最多处理 500 轮。
	rows, err := tx.Query(ctx, `SELECT id FROM runs WHERE `+terminalRunSQL+` AND ended_at<$1 AND EXISTS(SELECT 1 FROM run_events e WHERE e.run_id=runs.id) ORDER BY ended_at,id LIMIT 500 FOR UPDATE SKIP LOCKED`, eventCutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) > 0 && s.SnapshotParts == nil {
		return 0, fmt.Errorf("删除运行事件前必须配置过程快照")
	}
	for _, id := range ids {
		if err := s.SnapshotParts(ctx, tx, id); err != nil {
			return 0, err
		}
		var unsafe bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE run_id=$1 AND role='assistant' AND (parts IS NULL OR parts='[]'::jsonb)) OR (NOT EXISTS(SELECT 1 FROM messages WHERE run_id=$1 AND role='assistant') AND EXISTS(SELECT 1 FROM run_events WHERE run_id=$1 AND type IN ('message.delta','reasoning.delta','reasoning.completed','tool.preparing','tool.called','tool.started','tool.completed','approval.required','approval.resolved')))`, id).Scan(&unsafe); err != nil {
			return 0, err
		}
		if unsafe {
			return 0, fmt.Errorf("运行 %s 的可见过程尚未完成快照", id)
		}
	}
	var count int64
	if len(ids) > 0 {
		tag, err := tx.Exec(ctx, `DELETE FROM run_events WHERE run_id=ANY($1)`, ids)
		if err != nil {
			return 0, err
		}
		count += tag.RowsAffected()
	}
	for _, operation := range []struct {
		sql    string
		cutoff time.Time
	}{
		{`DELETE FROM run_contexts WHERE run_id IN (SELECT c.run_id FROM run_contexts c JOIN runs ON runs.id=c.run_id WHERE ` + terminalRunSQL + ` AND runs.ended_at<$1 ORDER BY runs.ended_at,runs.id LIMIT 500)`, contextCutoff},
		{`UPDATE run_spans SET attributes='{}',input_preview='',output_preview='' WHERE id IN (SELECT s.id FROM run_spans s JOIN runs ON runs.id=s.run_id WHERE ` + terminalRunSQL + ` AND runs.ended_at<$1 AND (s.attributes<>'{}'::jsonb OR s.input_preview<>'' OR s.output_preview<>'') ORDER BY runs.ended_at,s.id LIMIT 500)`, traceCutoff},
		{`UPDATE tool_results SET data=NULL WHERE id IN (SELECT t.id FROM tool_results t JOIN runs ON runs.id=t.run_id WHERE ` + terminalRunSQL + ` AND runs.ended_at<$1 AND t.data IS NOT NULL ORDER BY runs.ended_at,t.id LIMIT 500)`, traceCutoff},
	} {
		tag, err := tx.Exec(ctx, operation.sql, operation.cutoff)
		if err != nil {
			return 0, err
		}
		count += tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}
