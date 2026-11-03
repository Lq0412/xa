// Package postgres 创建共享连接池，业务仓储直接使用 pgx 的查询和事务接口。
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/config"
)

// Open 建立并验证连接；启动进程不负责创建业务表或执行迁移。
func Open(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	options, err := pgxpool.ParseConfig(cfg.URL())
	if err != nil {
		// pgx 的解析错误可能包含连接串，不能把含密码的原文传给日志。
		return nil, fmt.Errorf("PostgreSQL 连接配置无效")
	}
	options.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("创建 PostgreSQL 连接池: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	return pool, nil
}
