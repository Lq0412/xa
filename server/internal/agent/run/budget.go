package run

import (
	"fmt"
	"sync"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/infra/provider"
)

// Budget 跟踪四类预算：模型轮次、工具调用、墙钟与 token。
type Budget struct {
	mu     sync.Mutex
	Limits definition.RuntimeLimits

	Turns                int
	ToolCalls            int
	TotalTokens          int
	WrapUpEntered        bool
	ConsecutiveFailSteps int
}

// NewBudget 用生效限制初始化预算。
func NewBudget(limits definition.RuntimeLimits) *Budget {
	return &Budget{Limits: limits}
}

// HardCheck 在墙钟耗尽时返回 budget_exhausted/wall_time。
func (b *Budget) HardCheck(activeMS int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Limits.MaxWallTimeMS > 0 && activeMS >= int64(b.Limits.MaxWallTimeMS) {
		return fmt.Errorf("wall_time")
	}
	return nil
}

// NeedWrapUp 判断是否应进入收尾轮。
func (b *Budget) needWrapUpLocked() bool {
	if b.WrapUpEntered {
		return false
	}
	if b.Limits.MaxModelTurns > 0 && b.Turns >= b.Limits.MaxModelTurns-1 {
		return true
	}
	if b.Limits.MaxToolCalls >= 0 && b.ToolCalls >= b.Limits.MaxToolCalls {
		return true
	}
	if b.ConsecutiveFailSteps >= 3 {
		return true
	}
	return false
}

func (b *Budget) NeedWrapUp() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.needWrapUpLocked()
}

// MarkWrapUp 标记已进入收尾轮。
func (b *Budget) MarkWrapUp() {
	b.mu.Lock()
	b.WrapUpEntered = true
	b.mu.Unlock()
}

// AddModelTurn 累计模型轮次与 token。
func (b *Budget) AddModelTurn(u provider.Usage) {
	b.mu.Lock()
	b.Turns++
	b.TotalTokens += u.PromptTokens + u.CompletionTokens
	b.mu.Unlock()
}

// AddToolBudget 累计计入预算的工具调用（含规则 4–8 拒绝）。
func (b *Budget) AddToolBudget() {
	b.mu.Lock()
	b.ToolCalls++
	b.mu.Unlock()
}

// RecordToolStepFailure 更新连续工具失败步数，供收尾熔断使用。
func (b *Budget) RecordToolStepFailure(allFailed bool) {
	b.mu.Lock()
	if allFailed {
		b.ConsecutiveFailSteps++
	} else {
		b.ConsecutiveFailSteps = 0
	}
	b.mu.Unlock()
}

// TokenPreCheck 在下一轮调用前检查 token；不足时触发收尾或硬停。
func (b *Budget) TokenPreCheck(messages []provider.Message, toolSchemaTokens, maxOutput int) (wrapUp bool, hardStop bool, adjustMaxOut int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	adjustMaxOut = maxOutput
	if b.Limits.MaxTotalTokens <= 0 {
		return b.needWrapUpLocked(), false, adjustMaxOut
	}
	est := estimateMessagesTokens(messages) + toolSchemaTokens + maxOutput
	remain := b.Limits.MaxTotalTokens - b.TotalTokens
	if est <= remain {
		return b.needWrapUpLocked(), false, adjustMaxOut
	}
	if remain >= 2000 {
		return true, false, min(maxOutput, remain)
	}
	return false, true, adjustMaxOut
}

func estimateMessagesTokens(messages []provider.Message) int {
	total := 0
	for _, m := range messages {
		total += runtime.EstimateTokens(m.Content)
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				total += runtime.EstimateTokens(tc.Name + tc.Arguments)
			}
		}
	}
	return total
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ActiveMS 根据开始时间计算当前活跃毫秒。
func ActiveMS(started time.Time) int64 {
	if started.IsZero() {
		return 0
	}
	return time.Since(started).Milliseconds()
}
