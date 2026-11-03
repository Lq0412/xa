package provider

import "context"

// Router 根据模型、语音和向量配置选择实际供应商。
type Router interface {
	Resolve(ctx context.Context, capability string, revision string) (any, error)
}
