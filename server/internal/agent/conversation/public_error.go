package conversation

import (
	"errors"
	"strings"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/infra/provider"
)

// PublicError 将内部错误映射为 v1 SSE 可展示的中文信息。
func PublicError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, runtime.ErrMessageTooLong) {
		return "消息太长，请缩短后重试"
	}
	if errors.Is(err, definition.ErrConflict) || strings.Contains(err.Error(), "对话正在生成") {
		return "对话正在生成回复，请稍后再试"
	}
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		switch pe.Kind {
		case provider.ProviderErrorAuth, provider.ProviderErrorBadRequest:
			return "模型服务配置错误，请联系管理员"
		case provider.ProviderErrorContentFilter:
			return "内容未通过安全审核"
		case provider.ProviderErrorRateLimited, provider.ProviderErrorUnavailable, provider.ProviderErrorTimeout:
			return "模型繁忙，请稍后重试"
		}
	}
	if strings.Contains(err.Error(), "已停止") {
		return "已停止生成"
	}
	return "暂时无法完成回复，请稍后重试"
}
