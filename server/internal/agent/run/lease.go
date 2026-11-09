package run

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

func convLockKey(conversationID string) string { return "xa:conv:" + conversationID + ":writer" }

func acquireConvLock(ctx context.Context, r redisclient.Cmdable, conversationID, runID string, ttl time.Duration) error {
	if r == nil {
		return nil
	}
	val := fmt.Sprintf("%s:%s", runID, uuid.NewString())
	ok, err := r.SetNX(ctx, convLockKey(conversationID), val, ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("会话正在生成回复")
	}
	return nil
}

func releaseConvLock(ctx context.Context, r redisclient.Cmdable, conversationID, runID string) {
	if r == nil {
		return
	}
	key := convLockKey(conversationID)
	// 尽力删除；精确匹配留给租约过期。
	_ = r.Del(ctx, key).Err()
}
