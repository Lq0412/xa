package run

import (
	"context"
	"fmt"
	"sync"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// Coordinator 追踪本进程上的活跃 run 与事件总线。
type Coordinator struct {
	mu      sync.RWMutex
	buses   map[string]*EventBus
	cancels map[string]context.CancelCauseFunc
}

// NewCoordinator 创建空协调器。
func NewCoordinator() *Coordinator {
	return &Coordinator{buses: map[string]*EventBus{}, cancels: map[string]context.CancelCauseFunc{}}
}

// Register 登记 run 的取消函数与总线。
func (c *Coordinator) Register(runID string, cancel context.CancelCauseFunc, bus *EventBus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buses[runID] = bus
	c.cancels[runID] = cancel
}

// Bus 返回活跃 run 的事件总线。
func (c *Coordinator) Bus(runID string) *EventBus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buses[runID]
}

// Finish 移除活跃登记并关闭总线。
func (c *Coordinator) Finish(runID string) {
	c.mu.Lock()
	bus := c.buses[runID]
	delete(c.buses, runID)
	delete(c.cancels, runID)
	c.mu.Unlock()
	if bus != nil {
		bus.Close()
	}
}

// CancelRun 请求取消：写 Redis 标志并取消上下文。
func (c *Coordinator) CancelRun(ctx context.Context, redis redisclient.Cmdable, runID, reason string) error {
	c.mu.RLock()
	cancel := c.cancels[runID]
	c.mu.RUnlock()
	if redis != nil {
		_ = redis.Set(ctx, "xa:run:"+runID+":cancel", reason, 10*time.Minute).Err()
	}
	if cancel != nil {
		cancel(fmt.Errorf("%s", reason))
	}
	return nil
}

// TouchPresence 刷新订阅者存活标记。
func TouchPresence(ctx context.Context, redis redisclient.Cmdable, runID, subID string) {
	if redis == nil {
		return
	}
	_ = redis.Set(ctx, "xa:run:"+runID+":attached:"+subID, "1", 20*time.Second).Err()
}

// HasPresence 是否仍有活跃订阅者（任意 attached 键存在）。
func HasPresence(ctx context.Context, redis redisclient.Cmdable, runID string) bool {
	if redis == nil {
		return false
	}
	iter := redis.Scan(ctx, 0, "xa:run:"+runID+":attached:*", 8).Iterator()
	for iter.Next(ctx) {
		return true
	}
	return false
}
