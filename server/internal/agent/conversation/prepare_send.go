package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
)

type preparedSend struct {
	Conversation Conversation
	Agent        definition.AgentDefinition
	Version      definition.AgentVersion
	Policy       definition.MemoryPolicy
	Start        run.ChatStartInput
	Segments     []byte
}

func (s *Service) prepareSend(ctx context.Context, tx pgx.Tx, p auth.Principal, id, content string, protocol int) (preparedSend, error) {
	if strings.TrimSpace(content) == "" {
		return preparedSend{}, fmt.Errorf("消息不能为空")
	}
	var c Conversation
	if err := s.DB.QueryRow(ctx, `SELECT id,workspace_id,user_id,agent_id,title,created_at,updated_at FROM conversations WHERE id=$1 AND workspace_id=$2 AND user_id=$3`, id, p.WorkspaceID, p.UserID).Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return preparedSend{}, fmt.Errorf("会话不存在")
		}
		return preparedSend{}, err
	}
	a, err := s.Agents.Get(ctx, p, c.AgentID)
	if err != nil {
		return preparedSend{}, err
	}
	if a.Status != definition.StatusActive {
		return preparedSend{}, definition.ErrForbidden
	}
	version, err := s.Agents.GetVersion(ctx, p, a.ID, a.CurrentVersionID)
	if err != nil {
		return preparedSend{}, err
	}
	policy, err := definition.ParseMemoryPolicy(version.Configuration.MemoryPolicy)
	if err != nil {
		return preparedSend{}, err
	}
	windowService := s.Memory
	if windowService == nil {
		windowService = &memory.Service{DB: s.DB}
	}
	if err := runtime.ValidateCurrent(content); err != nil {
		return preparedSend{}, err
	}
	window, err := windowService.Window(ctx, p, id, policy.HistoryBudgetTokens)
	if err != nil {
		return preparedSend{}, err
	}
	history := window.Messages
	visible := make([]run.VisibleMessage, 0, len(history))
	previousUser := ""
	for _, m := range history {
		visible = append(visible, run.VisibleMessage{ID: m.ID, Role: m.Role, Content: m.Content, RunID: m.RunID, Status: "completed"})
		if m.Role == "user" {
			previousUser = m.Content
		}
	}
	limits, err := definition.ParseRuntimeLimits(version.Configuration.RuntimeLimits)
	if err != nil {
		return preparedSend{}, err
	}
	runStore := run.NewPGStore(s.DB)
	projector := run.HistoryProjector{Store: runStore, Limits: limits}
	historyMessages, err := projector.Project(ctx, id, visible, policy.HistoryBudgetTokens)
	if err != nil {
		return preparedSend{}, err
	}
	if s.Registry == nil {
		return preparedSend{}, fmt.Errorf("工具注册表未配置")
	}
	toolset, err := s.Registry.Resolve(tool.ResolveInput{
		Bindings: version.Configuration.Tools, MemoryPolicy: policy, Principal: p, CanApprove: protocol >= 2,
	})
	if err != nil {
		return preparedSend{}, err
	}
	builder := runtime.Builder{}
	if s.Memory != nil {
		builder.Memory = &memory.Module{Service: s.Memory, Actor: p, AgentID: a.ID, Current: content + "\n" + previousUser, Policy: policy}
	}
	if len(toolset.Entries) > 0 {
		builder.Tools = runtime.Tools{Toolset: toolset}
	}
	var sessionSummary string
	var summaryID string
	var summaryVersion int
	if policy.Enabled {
		err = s.DB.QueryRow(ctx, `SELECT id,version,content FROM conversation_summaries WHERE conversation_id=$1 AND owner_user_id=$2 ORDER BY version DESC LIMIT 1`, id, p.UserID).Scan(&summaryID, &summaryVersion, &sessionSummary)
		if err != nil && err != pgx.ErrNoRows {
			return preparedSend{}, err
		}
		if len(history) > 0 && s.Memory != nil {
			if err = s.Memory.EnqueueSummaryTx(ctx, tx, p, id, history[0].ID, policy.SummaryTriggerTokens); err != nil {
				return preparedSend{}, err
			}
		}
	}
	contextResult, err := builder.Build(ctx, runtime.Input{
		PlatformRules: run.BuildPlatformRules("你是 xa 平台中的 Agent。遵守用户权限和隐私边界，清晰回答当前问题。"),
		SoulMD:        version.Configuration.SoulMD, History: historyMessages, Current: content, SessionSummary: sessionSummary,
	})
	if err != nil {
		return preparedSend{}, err
	}
	if window.Details.OverBudget {
		contextResult.Segments = slices.Insert(contextResult.Segments, len(contextResult.Segments)-1, window.Details)
	}
	for i := range contextResult.Segments {
		seg := &contextResult.Segments[i]
		if seg.Name == "session_summary" {
			seg.SummaryID = summaryID
			seg.SummaryVersion = summaryVersion
		}
		if seg.Name == "history" && len(history) > 0 {
			seg.FromMessageID = history[0].ID
			seg.UntilMessageID = history[len(history)-1].ID
		}
	}
	modelName := version.Configuration.Model
	if !slices.Contains(s.AllowedModels, modelName) {
		return preparedSend{}, fmt.Errorf("不支持的模型 %s: %w", modelName, definition.ErrInvalid)
	}
	if seg, err := run.ToolSchemaSegment(toolset); err == nil && seg.Name != "" {
		contextResult.Segments = append(contextResult.Segments, seg)
	}
	segments, err := json.Marshal(contextResult.Segments)
	if err != nil {
		return preparedSend{}, err
	}
	start := run.ChatStartInput{
		Principal: p, Conversation: run.ConversationRef{ID: id, Title: c.Title, AgentID: a.ID},
		Agent: a, Version: version, UserContent: content,
		InitialMessages: contextResult.Messages, Segments: segments, Toolset: toolset, Limits: limits,
		MemoryPolicy: policy, MemoryEnabled: policy.Enabled, AllowedModels: s.AllowedModels,
	}
	return preparedSend{Conversation: c, Agent: a, Version: version, Policy: policy, Start: start, Segments: segments}, nil
}
