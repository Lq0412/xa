package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_identity_agents.sql
var identityAgentsMigration string

// Migrate 创建当前批次需要的业务表。每个迁移在事务中执行，失败时不留下半成品。
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, identityAgentsMigration)
		return err
	}); err != nil {
		return fmt.Errorf("执行身份和 Agent 数据库迁移: %w", err)
	}
	return nil
}
