package run

import (
	"context"
	"os"
	"strconv"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/infra/provider"
)

// HistoryProjector 将近期带工具的 run 展开为模型协议历史。
type HistoryProjector struct {
	Store  Store
	Limits definition.RuntimeLimits
}

// Project 输入可见消息（升序），输出投影后的 provider.Message 列表。
func (hp HistoryProjector) Project(ctx context.Context, conversationID string, visible []VisibleMessage, budget int) ([]provider.Message, error) {
	if len(visible) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(visible))
	runByMsg := map[string]string{}
	for _, m := range visible {
		if m.Role == "assistant" && m.RunID != "" {
			runByMsg[m.ID] = m.RunID
			ids = append(ids, m.ID)
		}
	}
	msgRuns, err := hp.Store.GetMessageRunIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for k, v := range msgRuns {
		runByMsg[k] = v
	}
	k := hp.Limits.HistoryToolRuns
	if k <= 0 {
		k = 2
	}
	recentRuns, err := hp.Store.ListRecentCompletedToolRuns(ctx, conversationID, k)
	if err != nil {
		return nil, err
	}
	expand := map[string]struct{}{}
	for _, r := range recentRuns {
		expand[r.ID] = struct{}{}
	}
	toolTok := historyToolResultTokens()
	out := make([]provider.Message, 0, len(visible))
	for _, m := range visible {
		if m.Role == "assistant" {
			rid := m.RunID
			if rid == "" {
				rid = runByMsg[m.ID]
			}
			if _, ok := expand[rid]; ok && rid != "" {
				msgs, err := hp.Store.ListRunMessages(ctx, rid)
				if err != nil {
					return nil, err
				}
				for _, rm := range msgs {
					msg := provider.Message{Role: rm.Role, Content: rm.Content, ToolCallID: rm.ToolCallID, Name: rm.ToolName, ToolCalls: rm.ToolCalls}
					if rm.Role == "tool" {
						msg.Content = HistoryOmitToolContent(rm.Content, toolTok, "")
					}
					out = append(out, msg)
				}
				continue
			}
		}
		out = append(out, provider.Message{Role: m.Role, Content: m.Content})
	}
	for runtime.EstimateTokensFromMessages(out) > budget && len(recentRuns) > 0 {
		drop := recentRuns[0].ID
		recentRuns = recentRuns[1:]
		delete(expand, drop)
		out = out[:0]
		for _, m := range visible {
			if m.Role == "assistant" {
				rid := m.RunID
				if rid == "" {
					rid = runByMsg[m.ID]
				}
				if _, ok := expand[rid]; ok && rid != "" {
					msgs, err := hp.Store.ListRunMessages(ctx, rid)
					if err != nil {
						return nil, err
					}
					for _, rm := range msgs {
						msg := provider.Message{Role: rm.Role, Content: rm.Content, ToolCallID: rm.ToolCallID, Name: rm.ToolName, ToolCalls: rm.ToolCalls}
						if rm.Role == "tool" {
							msg.Content = HistoryOmitToolContent(rm.Content, toolTok, "")
						}
						out = append(out, msg)
					}
					continue
				}
			}
			out = append(out, provider.Message{Role: m.Role, Content: m.Content})
		}
	}
	return out, nil
}

func historyToolResultTokens() int {
	if v := os.Getenv("XA_HISTORY_TOOL_RESULT_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 300
}
