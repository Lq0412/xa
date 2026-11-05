package definition

import (
	"encoding/json"
	"fmt"
)

// MemoryPolicy 控制整个记忆插槽；缺省启用，旧版本也能读取共享 profile。
type MemoryPolicy struct {
	Enabled              bool    `json:"enabled"`
	CoreBudgetTokens     int     `json:"core_budget_tokens"`
	RecallTopK           int     `json:"recall_top_k"`
	MinRelevance         float64 `json:"min_relevance"`
	HistoryBudgetTokens  int     `json:"history_budget_tokens"`
	SummaryTriggerTokens int     `json:"summary_trigger_tokens"`
	SummaryBudgetTokens  int     `json:"summary_budget_tokens"`
	DecayEnabled         bool    `json:"decay_enabled"`
}

// ParseMemoryPolicy 对缺省字段应用默认值，拒绝无效预算。
func ParseMemoryPolicy(raw json.RawMessage) (MemoryPolicy, error) {
	p := MemoryPolicy{Enabled: true, CoreBudgetTokens: 800, RecallTopK: 5, MinRelevance: 0.35, HistoryBudgetTokens: 6000, SummaryTriggerTokens: 3000, SummaryBudgetTokens: 800, DecayEnabled: true}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return p, fmt.Errorf("memory_policy: %w", ErrInvalid)
		}
	}
	if p.CoreBudgetTokens == 0 {
		p.CoreBudgetTokens = 800
	}
	if p.RecallTopK == 0 {
		p.RecallTopK = 5
	}
	if p.HistoryBudgetTokens == 0 {
		p.HistoryBudgetTokens = 6000
	}
	if p.SummaryTriggerTokens == 0 {
		p.SummaryTriggerTokens = 3000
	}
	if p.SummaryBudgetTokens == 0 {
		p.SummaryBudgetTokens = 800
	}
	if p.CoreBudgetTokens < 1 || p.CoreBudgetTokens > 8000 || p.RecallTopK < 1 || p.RecallTopK > 50 || p.MinRelevance < 0 || p.MinRelevance > 1 || p.HistoryBudgetTokens < 100 || p.HistoryBudgetTokens > 50000 || p.SummaryTriggerTokens < 100 || p.SummaryBudgetTokens < 100 || p.SummaryBudgetTokens > 10000 {
		return p, ErrInvalid
	}
	return p, nil
}
