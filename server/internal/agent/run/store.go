package run

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Store 抽象 run 相关持久化；测试使用内存实现，生产使用 PG。
type Store interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)

	InsertRun(ctx context.Context, tx pgx.Tx, run Run) error
	UpdateRun(ctx context.Context, tx pgx.Tx, run Run) error
	GetRun(ctx context.Context, id string) (Run, error)

	NextOrdinal(ctx context.Context, tx pgx.Tx, runID string) (int, error)
	AppendRunMessage(ctx context.Context, tx pgx.Tx, msg RunMessage) error
	ListRunMessages(ctx context.Context, runID string) ([]RunMessage, error)

	InsertToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error
	UpdateToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error
	GetToolResult(ctx context.Context, id string) (ToolResultRow, error)
	GetToolResultByCallID(ctx context.Context, runID, callID string) (ToolResultRow, error)

	SaveRunContext(ctx context.Context, tx pgx.Tx, snap RunContextSnapshot) error
	LoadRunContext(ctx context.Context, runID string) (RunContextSnapshot, error)

	// ListRecentCompletedToolRuns 返回最近完成且调用过工具的 run，按创建时间降序。
	ListRecentCompletedToolRuns(ctx context.Context, conversationID string, limit int) ([]Run, error)
	GetMessageRunIDs(ctx context.Context, messageIDs []string) (map[string]string, error)

	// PendingCallsOfLastStep P4 审批恢复用；P3 恒为空。
	PendingCallsOfLastStep(ctx context.Context, runID string) ([]PlannedCall, error)
}
