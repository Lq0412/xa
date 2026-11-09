package run

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
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
	mu               sync.Mutex
	dbMu             sync.Mutex // 串行化同一对话事务内的 PG 写入，供并行只读工具批次使用
	Tx               pgx.Tx     // Send 事务；非空时所有 run_messages / tool_results 须在同一事务内写入
	Run              Run
	Toolset          tool.Toolset
	Messages         []provider.Message
	Step             int
	stepCallIndex    int // 本 step 内已调度工具序号，写入 tool_results.call_index
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

// appendMessage 在并行调度中安全追加模型视图消息。
func (rc *RunContext) appendMessage(m provider.Message) {
	rc.mu.Lock()
	rc.Messages = append(rc.Messages, m)
	rc.mu.Unlock()
}

// ModelMessages 在收尾轮追加 system 提示并返回当前消息列表。
func (rc *RunContext) ModelMessages(wrapUp bool) []provider.Message {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if !wrapUp {
		out := make([]provider.Message, len(rc.Messages))
		copy(out, rc.Messages)
		return out
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
func (rc *RunContext) nextOrdinal(ctx context.Context) (int, error) {
	return rc.Store.NextOrdinal(ctx, rc.Tx, rc.Run.ID)
}

func (rc *RunContext) appendRunMessage(ctx context.Context, msg RunMessage) error {
	if msg.Ordinal == 0 {
		ord, err := rc.nextOrdinal(ctx)
		if err != nil {
			return err
		}
		msg.Ordinal = ord
	}
	rc.dbMu.Lock()
	defer rc.dbMu.Unlock()
	return rc.Store.AppendRunMessage(ctx, rc.Tx, msg)
}

// withToolTx 在 Chat 事务或独立短事务中执行工具持久化。
func (rc *RunContext) withToolTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if rc.Tx != nil {
		rc.dbMu.Lock()
		defer rc.dbMu.Unlock()
		return fn(rc.Tx)
	}
	tx, err := rc.Store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

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
