package handler

import "context"

// Handler 处理一个任务类型，必须保证重复执行具有幂等性。
type Handler interface {
	Handle(ctx context.Context, taskID string, payload []byte) error
}
