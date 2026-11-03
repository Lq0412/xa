package user

import "context"

// User 表示平台用户。
type User struct {
	ID          string
	WorkspaceID string
	DisplayName string
}

// Repository 隔离用户数据访问。
type Repository interface {
	Find(ctx context.Context, userID string) (User, error)
	Save(ctx context.Context, user User) error
}

// Service 提供用户相关用例。
type Service interface {
	Get(ctx context.Context, userID string) (User, error)
	Create(ctx context.Context, user User) error
}
