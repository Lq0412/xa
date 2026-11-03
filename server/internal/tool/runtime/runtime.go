package runtime

import "context"

// Input 是工具调用的结构化输入。
type Input struct {
	Name        string
	Arguments   []byte
	RequestID   string
	UserID      string
	WorkspaceID string
}

// Result 是工具调用的结构化结果。
type Result struct {
	Content  []byte
	Metadata map[string]string
}

// Executor 负责参数校验、权限校验、执行和取消。
type Executor interface {
	Execute(ctx context.Context, input Input) (Result, error)
}
