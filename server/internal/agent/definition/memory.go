package definition

import (
	"encoding/json"
	"fmt"
)

// MemoryPolicy 控制整个记忆插槽；缺省启用，旧版本也能读取共享 profile。
type MemoryPolicy struct {
	Enabled          bool `json:"enabled"`
	CoreBudgetTokens int  `json:"core_budget_tokens"`
	RecallTopK       int  `json:"recall_top_k"`
}

// ParseMemoryPolicy 对缺省字段应用默认值，拒绝无效预算。
func ParseMemoryPolicy(raw json.RawMessage) (MemoryPolicy, error) {
	p := MemoryPolicy{Enabled: true, CoreBudgetTokens: 800, RecallTopK: 5}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return p, fmt.Errorf("memory_policy: %w", ErrInvalid)
		}
	}
	if p.CoreBudgetTokens < 1 || p.CoreBudgetTokens > 8000 || p.RecallTopK < 1 || p.RecallTopK > 50 {
		return p, ErrInvalid
	}
	return p, nil
}
