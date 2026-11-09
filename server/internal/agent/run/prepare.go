package run

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// InsertRunRow 在短事务内写入 runs 与 run_contexts。
func InsertRunRow(ctx context.Context, tx pgx.Tx, store Store, run Run, snap RunContextSnapshot, idempotencyKey, requestHash, leaseOwner string) error {
	if err := store.InsertRunExtended(ctx, tx, run, idempotencyKey, requestHash, leaseOwner); err != nil {
		return err
	}
	return store.SaveRunContext(ctx, tx, snap)
}
