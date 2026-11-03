// Package redis 配置短期状态客户端，不承担业务事实的持久化。
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/framework/config"
)

// Open 验证认证与连通性，禁止客户端自动重试可能有副作用的命令。
func Open(ctx context.Context, cfg config.RedisConfig) (*redisclient.Client, error) {
	client := redisclient.NewClient(&redisclient.Options{
		Addr: cfg.Address, Password: cfg.Password, MaxRetries: -1,
		DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
		ContextTimeoutEnabled: true,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, errors.Join(fmt.Errorf("连接 Redis: %w", err), client.Close())
	}
	return client, nil
}
