package auth

import "context"

// Principal 表示经过认证的调用主体。
type Principal struct {
	UserID      string
	WorkspaceID string
	Roles       []string
}

// Resolver 从请求上下文中解析调用主体。
type Resolver interface {
	Resolve(ctx context.Context) (Principal, error)
}

// Authorizer 校验主体对资源和动作的权限。
type Authorizer interface {
	Authorize(ctx context.Context, principal Principal, resource string, action string) error
}
