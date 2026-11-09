package run

import (
	"context"
	"linq-bs/server/internal/infra/provider"
)

// Engine 保留运行时内核的依赖边界；P0 不改变现有会话行为。
type Engine interface {
	Execute(ctx context.Context, rc *RunContext) (Outcome, error)
}

// RunContext 保留可重入调用的状态；循环与持久化装配属于后续阶段。
type RunContext struct {
	Messages []provider.Message
	Step     int
}

// OutcomeKind 表示终态或审批挂起状态。
type OutcomeKind string

// Outcome 是单次推进的结果，不承担循环执行。
type Outcome struct {
	Kind      OutcomeKind
	Code      string
	Message   string
	Truncated bool
}
