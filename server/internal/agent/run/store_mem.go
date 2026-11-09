package run

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MemStore 是 Store 的内存实现，仅供单元测试。
type MemStore struct {
	mu       sync.Mutex
	runs     map[string]Run
	messages map[string][]RunMessage
	tools    map[string]ToolResultRow
	contexts map[string]RunContextSnapshot
	msgRunID map[string]string
	ordinals map[string]int
	events   map[string][]StoredEvent
	lastSeq  map[string]int64
}

// NewMemStore 创建空内存存储。
func NewMemStore() *MemStore {
	return &MemStore{
		runs: map[string]Run{}, messages: map[string][]RunMessage{},
		tools: map[string]ToolResultRow{}, contexts: map[string]RunContextSnapshot{},
		msgRunID: map[string]string{}, ordinals: map[string]int{},
		events: map[string][]StoredEvent{}, lastSeq: map[string]int64{},
	}
}

type memTx struct{}

func (m *MemStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return &memTx{}, nil
}

func (t *memTx) Begin(ctx context.Context) (pgx.Tx, error) { return t, nil }
func (t *memTx) Commit(ctx context.Context) error          { return nil }
func (t *memTx) Rollback(ctx context.Context) error        { return nil }
func (t *memTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, fmt.Errorf("未实现")
}
func (t *memTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (t *memTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (t *memTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, fmt.Errorf("未实现")
}
func (t *memTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *memTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("未实现")
}
func (t *memTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (t *memTx) Conn() *pgx.Conn                                               { return nil }

var _ pgx.Tx = (*memTx)(nil)

func (m *MemStore) InsertRunExtended(ctx context.Context, tx pgx.Tx, run Run, idempotencyKey, requestHash, leaseOwner string) error {
	return m.InsertRun(ctx, tx, run)
}

func (m *MemStore) InsertRun(ctx context.Context, tx pgx.Tx, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *MemStore) UpdateRun(ctx context.Context, tx pgx.Tx, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *MemStore) GetRun(ctx context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, fmt.Errorf("run 不存在")
	}
	return r, nil
}

func (m *MemStore) NextOrdinal(ctx context.Context, _ pgx.Tx, runID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ordinals[runID]++
	return m.ordinals[runID], nil
}

func (m *MemStore) AppendRunMessage(ctx context.Context, tx pgx.Tx, msg RunMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg.ID == "" {
		msg.ID = "rm_" + uuid.NewString()
	}
	if msg.Ordinal == 0 {
		m.ordinals[msg.RunID]++
		msg.Ordinal = m.ordinals[msg.RunID]
	}
	m.messages[msg.RunID] = append(m.messages[msg.RunID], msg)
	return nil
}

func (m *MemStore) ListRunMessages(ctx context.Context, runID string) ([]RunMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := append([]RunMessage(nil), m.messages[runID]...)
	sort.Slice(items, func(i, j int) bool { return items[i].Ordinal < items[j].Ordinal })
	return items, nil
}

func (m *MemStore) InsertToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row.ID == "" {
		row.ID = NewResultRef()
	}
	m.tools[row.ID] = row
	return nil
}

func (m *MemStore) UpdateToolResult(ctx context.Context, tx pgx.Tx, row ToolResultRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[row.ID] = row
	return nil
}

func (m *MemStore) GetToolResult(ctx context.Context, id string) (ToolResultRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.tools[id]
	if !ok {
		return ToolResultRow{}, fmt.Errorf("未找到")
	}
	return r, nil
}

func (m *MemStore) GetToolResultByCallID(ctx context.Context, runID, callID string) (ToolResultRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.tools {
		if r.RunID == runID && r.CallID == callID {
			return r, nil
		}
	}
	return ToolResultRow{}, fmt.Errorf("未找到")
}

func (m *MemStore) SaveRunContext(ctx context.Context, tx pgx.Tx, snap RunContextSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contexts[snap.RunID] = snap
	return nil
}

func (m *MemStore) LoadRunContext(ctx context.Context, runID string) (RunContextSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contexts[runID], nil
}

func (m *MemStore) ListRecentCompletedToolRuns(ctx context.Context, conversationID string, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Run, 0)
	for _, r := range m.runs {
		if r.ConversationID == conversationID && r.Status == "completed" && r.ToolCalls > 0 {
			items = append(items, r)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (m *MemStore) GetMessageRunIDs(ctx context.Context, messageIDs []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for _, id := range messageIDs {
		if v, ok := m.msgRunID[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (m *MemStore) PendingCallsOfLastStep(ctx context.Context, runID string) ([]PlannedCall, error) {
	return nil, nil
}

func (m *MemStore) AppendRunEvent(ctx context.Context, runID string, seq int64, typ string, payload []byte, terminal bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.events[runID] {
		if e.Seq == seq {
			return nil
		}
	}
	m.events[runID] = append(m.events[runID], StoredEvent{Seq: seq, Type: typ, Payload: payload})
	return nil
}

func (m *MemStore) UpdateLastEventSeq(ctx context.Context, runID string, seq int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSeq[runID] = seq
	return nil
}

func (m *MemStore) ListRunEventsAfter(ctx context.Context, runID string, afterSeq int64) ([]StoredEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.events[runID]
	out := make([]StoredEvent, 0)
	for _, e := range all {
		if e.Seq > afterSeq {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// LinkMessageRun 测试辅助：关联 assistant 消息与 run。
func (m *MemStore) LinkMessageRun(messageID, runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgRunID[messageID] = runID
}

// SeedRun 测试辅助写入 run。
func (m *MemStore) SeedRun(r Run) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	m.runs[r.ID] = r
}
