package run

import (
	"encoding/json"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

const (
	OutcomeCompleted       OutcomeKind = "completed"
	OutcomeFailed          OutcomeKind = "failed"
	OutcomeCancelled       OutcomeKind = "cancelled"
	OutcomeBudgetExhausted OutcomeKind = "budget_exhausted"
	OutcomeWaitingApproval OutcomeKind = "waiting_approval"
)

// Run 是一次对话轮次的执行记录快照。
type Run struct {
	ID               string
	WorkspaceID      string
	OwnerUserID      string
	ConversationID   string
	AgentID          string
	AgentVersionID   string
	RootRunID        string
	UserMessageID    string
	FinalMessageID   string
	Provider         string
	Model            string
	StreamProtocol   int
	Status           string
	Limits           definition.RuntimeLimits
	Toolset          tool.Toolset
	Steps            int
	ModelTurns       int
	ToolCalls        int
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	TTFTMS           *int64
	LatencyMS        int64
	ActiveMS         int64
	ErrorCode        string
	Error            string
	StartedAt        time.Time
	EndedAt          *time.Time
}

// RunMessage 是模型协议层的一条 run_messages 行。
type RunMessage struct {
	ID           string
	RunID        string
	Step         int
	Ordinal      int
	Role         string
	Content      string
	ToolCalls    []provider.ToolCall
	ToolCallID   string
	ToolName     string
	FinishReason string
}

// ToolResultRow 对应 tool_results 表。
type ToolResultRow struct {
	ID             string
	RunID          string
	ConversationID string
	OwnerUserID    string
	Step           int
	CallIndex      int
	CallID         string
	ToolName       string
	ToolVersion    string
	Effect         string
	Arguments      json.RawMessage
	RawArguments   string
	ArgsHash       string
	Status         string
	ErrorCode      string
	ErrorMessage   string
	ModelContent   string
	Data           json.RawMessage
	Display        json.RawMessage
	Truncated      bool
	TotalChars     int
	Attempts       int
}

// RunContextSnapshot 持久化到 run_contexts。
type RunContextSnapshot struct {
	RunID    string
	Messages []provider.Message
	Segments json.RawMessage
	Resume   ResumeState
}

// VisibleMessage 供 HistoryProjector 输入。
type VisibleMessage struct {
	ID              string
	Role            string
	Content         string
	RunID           string
	Status          string
	EstimatedTokens int
}

// ChatStartInput 启动一次同步 chat run 的上下文。
type ChatStartInput struct {
	ContextStartedAt time.Time
	ContextEndedAt   time.Time
	ContextTrace     []ContextObservation
	Principal        auth.Principal
	Conversation     ConversationRef
	Agent            definition.AgentDefinition
	Version          definition.AgentVersion
	UserContent      string
	UserMessageID    string
	PlatformRules    string
	History          []VisibleMessage
	SessionSummary   string
	Segments         []byte
	InitialMessages  []provider.Message
	Toolset          tool.Toolset
	Limits           definition.RuntimeLimits
	MemoryPolicy     definition.MemoryPolicy
	MemoryEnabled    bool
	AllowedModels    []string
	EnableThinking   bool
}

// ContextObservation 保存准备阶段的计时，run 入库后再生成追踪节点。
type ContextObservation struct {
	Slot               string
	Segment            runtime.Segment
	StartedAt, EndedAt time.Time
}

// ConversationRef 避免 run 包依赖 conversation 的全部类型。
type ConversationRef struct {
	ID      string
	Title   string
	AgentID string
}

// FinalizeInput 收尾写入可见消息与 run 状态。
type FinalizeInput struct {
	Run                 Run
	VisibleText         string
	Outcome             Outcome
	Usage               provider.Usage
	LatencyMS           int64
	ContextSegments     []byte
	AssistantStatus     string
	AssistantError      string
	EnqueueMemoryUserID string
}
