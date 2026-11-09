package run

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// IdempotencyRef 按 conversation + idempotency_key 查到的 run。
type IdempotencyRef struct {
	RunID       string
	RequestHash string
	Status      string
}

// LookupIdempotency 查找幂等键对应的 run；不存在返回 pgx.ErrNoRows。
func (s *PGStore) LookupIdempotency(ctx context.Context, conversationID, key string) (IdempotencyRef, error) {
	var ref IdempotencyRef
	err := s.DB.QueryRow(ctx, `
SELECT id, COALESCE(request_hash,''), status
FROM runs
WHERE conversation_id=$1 AND idempotency_key=$2 AND parent_run_id IS NULL
ORDER BY created_at DESC
LIMIT 1`, conversationID, key).Scan(&ref.RunID, &ref.RequestHash, &ref.Status)
	if err != nil {
		return IdempotencyRef{}, err
	}
	return ref, nil
}

// ActiveRunInfo 会话级进行中 run，供前端续传。
type ActiveRunInfo struct {
	ID           string
	Status       string
	LastEventSeq int64
}

// ActiveRunForConversation 返回当前活跃顶层 run（最多一条）。
func (s *PGStore) ActiveRunForConversation(ctx context.Context, conversationID, ownerUserID string) (*ActiveRunInfo, error) {
	var info ActiveRunInfo
	err := s.DB.QueryRow(ctx, `
SELECT id, status, last_event_seq
FROM runs
WHERE conversation_id=$1 AND owner_user_id=$2 AND parent_run_id IS NULL
  AND status IN ('running','waiting_approval')
ORDER BY created_at DESC
LIMIT 1`, conversationID, ownerUserID).Scan(&info.ID, &info.Status, &info.LastEventSeq)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}
