package model

import "context"

// Factory 根据模型配置创建供应商客户端。
type Factory interface {
	Create(ctx context.Context, modelName string) (Client, error)
}
