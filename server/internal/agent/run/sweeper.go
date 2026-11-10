package run

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sweeper 周期性回收僵尸 run。
type Sweeper struct {
	DB       *pgxpool.Pool
	Interval time.Duration
	Stale    time.Duration
	Runs     *RunService
}

// Run 阻塞运行 sweeper，直到 ctx 取消。
func (sw *Sweeper) Run(ctx context.Context) {
	if sw.Interval <= 0 {
		sw.Interval = 15 * time.Second
	}
	if sw.Stale <= 0 {
		sw.Stale = 30 * time.Second
	}
	ticker := time.NewTicker(sw.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sw.tick(ctx)
		}
	}
}

// TickOnce 执行一轮回收，供集成测试调用。
func (sw *Sweeper) TickOnce(ctx context.Context) {
	sw.tick(ctx)
}

func (sw *Sweeper) tick(ctx context.Context) {
	if sw.Runs != nil {
		if err := sw.Runs.ExpireApprovals(ctx); err != nil {
			slog.Error("审批超时处理失败", "error", err)
		}
	}
	var locked bool
	if err := sw.DB.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('xa_run_sweeper'))`).Scan(&locked); err != nil || !locked {
		return
	}
	defer func() { _, _ = sw.DB.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('xa_run_sweeper'))`) }()
	rows, err := sw.DB.Query(ctx, `
SELECT id, conversation_id, owner_user_id FROM runs
WHERE status='running' AND lease_expires_at < now() - make_interval(secs => $1)`, sw.Stale.Seconds())
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, conv, owner string
		if err := rows.Scan(&id, &conv, &owner); err != nil {
			continue
		}
		sw.interruptRun(ctx, id, conv, owner)
	}
}

func (sw *Sweeper) interruptRun(ctx context.Context, runID, convID, owner string) {
	tx, err := sw.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var updated int64
	if err := tx.QueryRow(ctx, `
UPDATE runs SET status='interrupted',error='服务中断，本次回复未完成',error_code='interrupted',ended_at=now(),updated_at=now()
WHERE id=$1 AND status='running'
RETURNING 1`, runID).Scan(&updated); err != nil || updated == 0 {
		return
	}
	_, _ = tx.Exec(ctx, `UPDATE tool_results SET status='unknown' WHERE run_id=$1 AND status='running' AND effect!='read'`, runID)
	_, _ = tx.Exec(ctx, `UPDATE tool_results SET status='cancelled' WHERE run_id=$1 AND status='running' AND effect='read'`, runID)
	_, _ = tx.Exec(ctx, `UPDATE run_approvals SET status='cancelled',decided_at=now() WHERE run_id=$1 AND status='pending'`, runID)
	_ = tx.Commit(ctx)
}
