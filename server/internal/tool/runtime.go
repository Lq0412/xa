package tool

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/framework/auth"
)

type Effect string

const (
	EffectRead        Effect = "read"
	EffectWrite       Effect = "write"
	EffectDestructive Effect = "destructive"
)

type ApprovalMode string

const (
	ApprovalNever  ApprovalMode = "never"
	ApprovalAlways ApprovalMode = "always"
)

type ApprovalPolicy struct {
	Default     ApprovalMode
	Overridable bool
}

// Spec 是工具注册表中的稳定描述，当前阶段只提供类型契约。
type Spec struct {
	Name            string
	Version         string
	Title           string
	Description     string
	InputSchema     json.RawMessage
	ConfigSchema    json.RawMessage
	Effect          Effect
	Idempotent      bool
	DefaultEnabled  bool
	Approval        ApprovalPolicy
	DefaultTimeout  time.Duration
	MaxCallsPerRun  int
	ResultMaxTokens int
	Requires        []Requirement
	Available       bool
}

type Call struct {
	ID        string
	Name      string
	Arguments json.RawMessage
	Step      int
}

type Env struct {
	Principal      auth.Principal
	WorkspaceID    string
	AgentID        string
	AgentVersionID string
	IsPrimaryAgent bool
	ConversationID string
	RunID          string
	UserMessageID  string
	Config         json.RawMessage
	IdempotencyKey string
	Deadline       time.Time
	Tx             pgx.Tx
}

type Result struct {
	Data    any
	Display Display
	Effects []Effect2
}

type Display struct {
	Summary string        `json:"summary"`
	Items   []DisplayItem `json:"items,omitempty"`
}

type DisplayItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Effect2 struct {
	Kind     string            `json:"kind"`
	TargetID string            `json:"target_id"`
	Attrs    map[string]string `json:"attrs,omitempty"`
}

type Error struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *Error) Error() string { return e.Message }

type Tool interface {
	Spec() Spec
	Execute(context.Context, Call, Env) (Result, error)
}

type TxTool interface {
	Tool
	Prepare(context.Context, Call, Env) (any, error)
	ExecuteTx(context.Context, pgx.Tx, Call, Env, any) (Result, error)
}

type Summarizer interface {
	ModelView(Result, int) (string, bool)
}

// Previewable 在执行前读取审批摘要，不产生副作用。
type Previewable interface {
	Preview(context.Context, Call, Env) (Display, error)
}
