package audit

import "context"

// Event 记录管理操作和关键运行操作。
type Event struct {
	ID          string
	ActorID     string
	WorkspaceID string
	Action      string
	ResourceID  string
	Metadata    map[string]any
}

// Service 提供审计事件写入边界。
type Service interface {
	Record(ctx context.Context, event Event) error
}
