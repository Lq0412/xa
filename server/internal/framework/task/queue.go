package task

import "context"

// Task 描述一个需要异步执行的业务任务。
type Task struct {
	ID             string
	Kind           string
	IdempotencyKey string
	Payload        []byte
}

// Queue 是异步任务投递边界。
type Queue interface {
	Enqueue(ctx context.Context, task Task) error
	Receive(ctx context.Context) (Task, error)
	Ack(ctx context.Context, taskID string) error
	Fail(ctx context.Context, taskID string, cause error) error
}
