package run

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

func convLockKey(conversationID string) string { return "xa:conv:" + conversationID + ":writer" }

func acquireConvLock(ctx context.Context, r redisclient.Cmdable, conversationID, runID string, ttl time.Duration) (string, error) {
	if r == nil {
		return "", nil
	}
	val := runID + ":" + uuid.NewString()
	ok, err := r.SetNX(ctx, convLockKey(conversationID), val, ttl).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("会话正在生成回复")
	}
	return val, nil
}

func releaseConvLock(ctx context.Context, r redisclient.Cmdable, conversationID, token string) {
	if r == nil || token == "" {
		return
	}
	// 只释放本次执行取得的锁，避免旧租约清除续跑进程的锁。
	_ = r.Eval(ctx, `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end`, []string{convLockKey(conversationID)}, token).Err()
}
