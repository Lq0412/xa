package runtime

import "context"

// Runner 负责把会话上下文交给 AgentProvider，并转换运行事件。
type Runner struct {
	Provider AgentProvider
}

// NewRunner 创建运行入口，依赖由 bootstrap 注入。
func NewRunner(provider AgentProvider) *Runner {
	return &Runner{Provider: provider}
}

// Run 执行一次 ReAct 运行。
func (r *Runner) Run(ctx context.Context, input RunInput) (<-chan Event, error) {
	panic("TODO: implement agent runner")
}
