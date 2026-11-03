package prompt

import "context"

// ResolveInput 描述一次提示词解析所需的配置和变量。
type ResolveInput struct {
	VersionID string
	Slots     []Slot
	Variables map[string]string
}

// Resolver 校验提示词变量并生成可运行的提示词快照。
type Resolver interface {
	Resolve(ctx context.Context, input ResolveInput) (Snapshot, error)
}
