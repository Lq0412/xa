package definition

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RuntimeLimits 控制单次 run 的资源预算；缺省时使用设计默认值。
type RuntimeLimits struct {
	MaxModelTurns    int `json:"max_model_turns"`
	MaxToolCalls     int `json:"max_tool_calls"`
	MaxParallelTools int `json:"max_parallel_tools"`
	MaxCallsPerStep  int `json:"max_calls_per_step"`
	MaxWallTimeMS    int `json:"max_wall_time_ms"`
	MaxTotalTokens   int `json:"max_total_tokens"`
	MaxOutputTokens  int `json:"max_output_tokens"`
	ToolResultMaxTok int `json:"tool_result_max_tokens"`
	HistoryToolRuns  int `json:"history_tool_runs"`
}

// ParseRuntimeLimits 解析并校验 runtime_limits JSON。
func ParseRuntimeLimits(raw json.RawMessage) (RuntimeLimits, error) {
	limits := defaultRuntimeLimits()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &limits); err != nil {
			return limits, fmt.Errorf("runtime_limits: %w", ErrInvalid)
		}
	}
	if err := (&limits).validate(); err != nil {
		return limits, err
	}
	return limits, nil
}

func defaultRuntimeLimits() RuntimeLimits {
	return RuntimeLimits{
		MaxModelTurns: 8, MaxToolCalls: 16, MaxParallelTools: 4, MaxCallsPerStep: 8,
		MaxWallTimeMS: 120000, MaxTotalTokens: 64000, MaxOutputTokens: 2048,
		ToolResultMaxTok: 1500, HistoryToolRuns: 2,
	}
}

func (l *RuntimeLimits) validate() error {
	capTurns := envInt("XA_RUN_MAX_MODEL_TURNS_CAP", 20)
	capTools := envInt("XA_RUN_MAX_TOOL_CALLS_CAP", 64)
	capParallel := envInt("XA_RUN_MAX_PARALLEL_CAP", 8)
	capWall := envInt("XA_RUN_MAX_WALL_CAP", 600) * 1000
	capTokens := envInt("XA_RUN_MAX_TOKENS_CAP", 400000)
	if l.MaxModelTurns == 0 {
		l.MaxModelTurns = 8
	}
	if l.MaxToolCalls == 0 {
		l.MaxToolCalls = 16
	}
	if l.MaxParallelTools == 0 {
		l.MaxParallelTools = 4
	}
	if l.MaxCallsPerStep == 0 {
		l.MaxCallsPerStep = 8
	}
	if l.MaxWallTimeMS == 0 {
		l.MaxWallTimeMS = 120000
	}
	if l.MaxTotalTokens == 0 {
		l.MaxTotalTokens = 64000
	}
	if l.MaxOutputTokens == 0 {
		l.MaxOutputTokens = 2048
	}
	if l.ToolResultMaxTok == 0 {
		l.ToolResultMaxTok = 1500
	}
	if l.HistoryToolRuns == 0 {
		l.HistoryToolRuns = 2
	}
	if l.MaxModelTurns < 1 || l.MaxModelTurns > capTurns || l.MaxToolCalls < 0 || l.MaxToolCalls > capTools ||
		l.MaxParallelTools < 1 || l.MaxParallelTools > capParallel || l.MaxCallsPerStep < 1 || l.MaxCallsPerStep > 16 ||
		l.MaxWallTimeMS < 10000 || l.MaxWallTimeMS > capWall ||
		l.MaxTotalTokens < 4000 || l.MaxTotalTokens > capTokens ||
		l.MaxOutputTokens < 256 || l.MaxOutputTokens > 8192 ||
		l.ToolResultMaxTok < 200 || l.ToolResultMaxTok > 8000 ||
		l.HistoryToolRuns < 0 || l.HistoryToolRuns > 5 {
		return ErrInvalid
	}
	return nil
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
