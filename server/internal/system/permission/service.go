package permission

import "context"

// Service 负责资源权限判定。
type Service interface {
	Check(ctx context.Context, subjectID string, resource string, action string) error
}
