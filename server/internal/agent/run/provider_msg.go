package run

import "linq-bs/server/internal/infra/provider"

func toolMessage(name, callID, content string) provider.Message {
	return provider.Message{Role: "tool", Name: name, ToolCallID: callID, Content: content}
}
