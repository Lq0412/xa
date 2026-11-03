package postgres

import "context"

// Client 表示 PostgreSQL 连接和事务能力的边界。
type Client interface {
	Ping(ctx context.Context) error
	Begin(ctx context.Context) (Tx, error)
	Close(ctx context.Context) error
}

// Tx 表示一个业务事务。
type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
