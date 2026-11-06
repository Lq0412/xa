// Package runtime 按固定顺序组装模型上下文，模块不能改变彼此的权限或顺序。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"linq-bs/server/internal/infra/provider"
	"strings"
	"unicode"
)

// Segment 记录每一段来源与估算成本，估算值不替代供应商实际计费 token。
type Segment struct {
	Name                string    `json:"name"`
	Role                string    `json:"role"`
	EstimatedTokens     int       `json:"estimated_tokens"`
	ItemIDs             []string  `json:"item_ids,omitempty"`
	Scores              []float64 `json:"scores,omitempty"`
	SummaryID           string    `json:"summary_id,omitempty"`
	SummaryVersion      int       `json:"summary_version,omitempty"`
	FromMessageID       string    `json:"from_message_id,omitempty"`
	UntilMessageID      string    `json:"until_message_id,omitempty"`
	OverBudget          bool      `json:"over_budget,omitempty"`
	Truncated           bool      `json:"truncated,omitempty"`
	DroppedRounds       int       `json:"dropped_rounds,omitempty"`
	TruncatedMessageIDs []string  `json:"truncated_message_ids,omitempty"`
}

// TruncateTokens 按 Unicode 边界截断，并把提示本身计入预算。
func TruncateTokens(content string, budget int) string {
	if EstimateTokens(content) <= budget {
		return content
	}
	const suffix = "…（已截断）"
	if budget < EstimateTokens(suffix) {
		return ""
	}
	runes := []rune(content)
	left, right := 0, len(runes)
	for left < right {
		mid := (left + right + 1) / 2
		if EstimateTokens(string(runes[:mid])+suffix) <= budget {
			left = mid
		} else {
			right = mid - 1
		}
	}
	return string(runes[:left]) + suffix
}

// ContextModule 是记忆、知识库和工具说明的读取边界。
type ContextModule interface {
	Enabled() bool
	Context(context.Context) ([]string, error)
}

// Input 只包含当前轮次已经授权的数据。
type Input struct {
	PlatformRules  string
	SoulMD         string
	History        []provider.Message
	Current        string
	SessionSummary string
}

// ModelContextTokens 是当前百炼文本模型的上下文上限，沿用统一 token 估算口径。
const ModelContextTokens = 1000000

// ErrMessageTooLong 只用于当前消息自身无法放入模型上下文的情况。
var ErrMessageTooLong = errors.New("消息太长，超出模型可处理的长度，请缩短或分段发送")

// ValidateCurrent 在读取记忆或调用模型之前检查当前消息，历史超预算不返回此错误。
func ValidateCurrent(content string) error {
	if EstimateTokens(content) > ModelContextTokens {
		return ErrMessageTooLong
	}
	return nil
}

// Result 同时返回模型消息与可持久化的分段用量。
type Result struct {
	Segments             []Segment
	Messages             []provider.Message
	TotalEstimatedTokens int
}

// Builder 的插槽顺序固定；nil 或未启用的模块直接跳过。
type Builder struct {
	Memory    ContextModule
	Knowledge ContextModule
	Tools     ContextModule
}

// Build 在模块错误发生位置返回，不以空内容掩盖失败。
func (b Builder) Build(ctx context.Context, input Input) (Result, error) {
	if err := ValidateCurrent(input.Current); err != nil {
		return Result{}, err
	}
	result := Result{Segments: []Segment{}, Messages: []provider.Message{}}
	add := func(name, role, content string) {
		if strings.TrimSpace(content) == "" {
			return
		}
		segment := Segment{Name: name, Role: role, EstimatedTokens: estimateTokens(content)}
		result.Segments = append(result.Segments, segment)
		result.Messages = append(result.Messages, provider.Message{Role: role, Content: content})
		result.TotalEstimatedTokens += segment.EstimatedTokens
	}
	add("platform", "system", input.PlatformRules)
	add("soul", "system", input.SoulMD)
	for _, slot := range []struct {
		name   string
		module ContextModule
	}{{"memory", b.Memory}, {"knowledge", b.Knowledge}, {"tools", b.Tools}} {
		if slot.module == nil || !slot.module.Enabled() {
			continue
		}
		parts, err := slot.module.Context(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("读取 %s 上下文: %w", slot.name, err)
		}
		var details []Segment
		if module, ok := slot.module.(interface{ ContextSegments() []Segment }); ok {
			details = module.ContextSegments()
		}
		for i, part := range parts {
			add(slot.name, "system", part)
			if i < len(details) && strings.TrimSpace(part) != "" {
				segment := details[i]
				segment.Role = "system"
				segment.EstimatedTokens = EstimateTokens(part)
				result.Segments[len(result.Segments)-1] = segment
			}
		}
	}
	add("session_summary", "system", input.SessionSummary)
	for _, message := range input.History {
		add("history", message.Role, message.Content)
	}
	add("current", "user", input.Current)
	return result, nil
}

// 中文按约一字一个 token、ASCII 按约四字一个 token 估算，保留各段独立开销。
func estimateTokens(content string) int {
	return EstimateTokens(content)
}

// EstimateTokens 供记忆预算与 Builder 共用相同估算口径。
func EstimateTokens(content string) int {
	ascii, other := 0, 0
	for _, r := range content {
		if r <= unicode.MaxASCII {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}
