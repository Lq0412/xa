package mcp

import "context"

// Client 是 MCP 工具服务的协议适配边界。
type Client interface {
	ListTools(ctx context.Context) ([]Tool, error)
	Call(ctx context.Context, name string, arguments []byte) ([]byte, error)
	Close(ctx context.Context) error
}

// Tool 是 MCP 服务发现的工具描述。
type Tool struct {
	Name        string
	Description string
	Schema      []byte
}
