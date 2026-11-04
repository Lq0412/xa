package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Migrate 执行尚未记录的数据库迁移，每个迁移在事务中执行。
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	names, err := migrationNames()
	if err != nil {
		return fmt.Errorf("读取数据库迁移: %w", err)
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
			return err
		}
		for _, name := range names {
			var applied bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&applied); err != nil {
				return err
			}
			if applied {
				continue
			}
			content, err := migrationFiles.ReadFile("migrations/" + name)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, string(content)); err != nil {
				return fmt.Errorf("执行迁移 %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("执行数据库迁移: %w", err)
	}
	return nil
}

// CheckMigrations 在进程启动时确认数据库已由独立 migrate 命令更新到最新版本。
func CheckMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	names, err := migrationNames()
	if err != nil {
		return fmt.Errorf("读取数据库迁移: %w", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		return fmt.Errorf("读取数据库迁移版本: %w", err)
	}
	if count != len(names) {
		return fmt.Errorf("数据库迁移版本不是最新，请先运行 migrate 命令")
	}
	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("读取数据库迁移版本: %w", err)
		}
		if !applied {
			return fmt.Errorf("数据库迁移版本不是最新，请先运行 migrate 命令")
		}
	}
	return nil
}
