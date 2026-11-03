package definition

// Status 表示异步任务状态。
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusDead      Status = "dead"
)

// Task 是可重试且带幂等键的异步任务。
type Task struct {
	ID             string
	Kind           string
	IdempotencyKey string
	Payload        []byte
	Status         Status
}
