// Package runtime 按固定顺序组装模型上下文，模块不能改变彼此的权限或顺序。
package runtime

import (
	"context"
	"fmt"
	"linq-bs/server/internal/infra/provider"
	"strings"
	"unicode"
)

// Segment 记录每一段来源与估算成本，估算值不替代供应商实际计费 token。
type Segment struct {
	Name            string `json:"name"`
	Role            string `json:"role"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// ContextModule 是记忆、知识库和工具说明的读取边界；本次不注册这些模块。
type ContextModule interface {
	Enabled() bool
	Context(context.Context) ([]string, error)
}

// Input 只包含当前轮次已经授权的数据。
type Input struct {
	PlatformRules string
	SoulMD        string
	History       []provider.Message
	Current       string
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
		for _, part := range parts {
			add(slot.name, "system", part)
		}
	}
	for _, message := range input.History {
		add("history", message.Role, message.Content)
	}
	add("current", "user", input.Current)
	return result, nil
}

// 中文按约一字一个 token、ASCII 按约四字一个 token 估算，保留各段独立开销。
func estimateTokens(content string) int {
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
