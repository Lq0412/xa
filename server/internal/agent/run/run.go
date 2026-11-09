package run

import (
	"context"
	"strings"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

// OutcomeKind 表示 run 终态；P3 不使用 waiting_approval。
type OutcomeKind string

// Engine 从当前 RunContext 推进到终态；Resume 时可重入。
type Engine interface {
	Execute(ctx context.Context, rc *RunContext) (Outcome, error)
}

// RunContext 承载单次 run 的可变执行状态。
type RunContext struct {
	Run              Run
	Toolset          tool.Toolset
	Messages         []provider.Message
	Step             int
	Budget           *Budget
	VisibleText      strings.Builder
	Store            Store
	Emitter          *Emitter
	Provider         provider.ChatProvider
	Registry         *tool.Registry
	Env              tool.Env
	Limits           definition.RuntimeLimits
	ReliableFnChoice bool
	Emitted          bool
	WallStarted      time.Time
	CallHashes       map[string]int
	PerToolCalls     map[string]int
	FailedSteps      int
}

// Outcome 是 Execute 的返回值，Kind 为终态或挂起类别。
type Outcome struct {
	Kind      OutcomeKind
	Code      string
	Message   string
	Truncated bool
}

// ModelMessages 在收尾轮追加 system 提示并返回当前消息列表。
func (rc *RunContext) ModelMessages(wrapUp bool) []provider.Message {
	if !wrapUp {
		return rc.Messages
	}
	out := make([]provider.Message, len(rc.Messages), len(rc.Messages)+1)
	copy(out, rc.Messages)
	out = append(out, provider.Message{
		Role:    "system",
		Content: "工具预算已用尽，请基于已有信息直接回答，不要再调用工具。",
	})
	return out
}

// RemainingWall 返回距离墙钟预算结束的剩余时间。
func (rc *RunContext) RemainingWall() time.Duration {
	if rc.WallStarted.IsZero() {
		return time.Duration(rc.Limits.MaxWallTimeMS) * time.Millisecond
	}
	elapsed := time.Since(rc.WallStarted)
	remain := time.Duration(rc.Limits.MaxWallTimeMS)*time.Millisecond - elapsed
	if remain < 0 {
		return 0
	}
	return remain
}
