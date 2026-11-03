package redis

import "context"

// Client 表示 Redis 的短期状态、锁和缓存能力边界。
type Client interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttlSeconds int64) error
	Delete(ctx context.Context, key string) error
	Close(ctx context.Context) error
}
