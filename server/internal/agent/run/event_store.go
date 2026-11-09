package run

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// AppendRunEvent 写入 run_events；冲突时忽略（幂等 seq）。
func (s *PGStore) AppendRunEvent(ctx context.Context, runID string, seq int64, typ string, payload []byte, terminal bool) error {
	_, err := s.DB.Exec(ctx, `
INSERT INTO run_events(run_id,seq,type,payload) VALUES($1,$2,$3,$4)
ON CONFLICT (run_id, seq) DO NOTHING`, runID, seq, typ, payload)
	return err
}

// UpdateLastEventSeq 更新 runs.last_event_seq。
func (s *PGStore) UpdateLastEventSeq(ctx context.Context, runID string, seq int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE runs SET last_event_seq=$2, updated_at=now() WHERE id=$1`, runID, seq)
	return err
}

// ListRunEventsAfter 按 seq 递增读取事件，供重放。
func (s *PGStore) ListRunEventsAfter(ctx context.Context, runID string, afterSeq int64) ([]StoredEvent, error) {
	rows, err := s.DB.Query(ctx, `
SELECT seq,type,payload FROM run_events WHERE run_id=$1 AND seq>$2 ORDER BY seq ASC`, runID, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredEvent
	for rows.Next() {
		var ev StoredEvent
		if err := rows.Scan(&ev.Seq, &ev.Type, &ev.Payload); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// FenceRunLease 校验 lease_owner 并锁定 run 行（fencing）。
func (s *PGStore) FenceRunLease(ctx context.Context, tx pgx.Tx, runID, owner string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT lease_owner=$2 FROM runs WHERE id=$1 FOR UPDATE`, runID, owner).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}
