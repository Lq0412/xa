package eino

import (
	"context"

	"linq-bs/server/internal/agent/runtime"
)

// Provider 是 Eino ChatModelAgent 的适配器，Eino 类型只在本包内出现。
type Provider struct{}

// NewProvider 创建 Eino 运行实例提供器。
func NewProvider() *Provider {
	return &Provider{}
}

// Get 按 Agent 版本和配置指纹获取运行实例。
func (p *Provider) Get(ctx context.Context, input runtime.ProviderInput) (runtime.Agent, error) {
	panic("TODO: build or reuse Eino ChatModelAgent")
}
