package system

// ResourceOwner 描述需要进行工作空间隔离的资源。
type ResourceOwner struct {
	WorkspaceID string
	UserID      string
}

// PermissionChecker 是业务模块使用的权限校验接口。
type PermissionChecker interface {
	Can(resourceID string, action string) error
}
