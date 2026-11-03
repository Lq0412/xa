package definition

import "errors"

var (
	ErrNotFound  = errors.New("Agent 不存在")
	ErrConflict  = errors.New("Agent 配置已被其他请求更新")
	ErrForbidden = errors.New("无权管理此 Agent")
	ErrInvalid   = errors.New("Agent 配置无效")
)
