package conversation

import (
	"errors"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/infra/provider"
)

func TestPublicErrorWhitelist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "message_too_long", err: runtime.ErrMessageTooLong, want: "消息太长，请缩短后重试"},
		{name: "conflict", err: definition.ErrConflict, want: "对话正在生成回复，请稍后再试"},
		{name: "generating", err: errors.New("对话正在生成回复，请稍后再试"), want: "对话正在生成回复，请稍后再试"},
		{name: "provider_auth", err: &provider.ProviderError{Kind: provider.ProviderErrorAuth, Message: "sk-secret-key-xxx"}, want: "模型服务配置错误，请联系管理员"},
		{name: "content_filter", err: &provider.ProviderError{Kind: provider.ProviderErrorContentFilter, Message: "blocked"}, want: "内容未通过安全审核"},
		{name: "rate_limit", err: &provider.ProviderError{Kind: provider.ProviderErrorRateLimited, Message: "429"}, want: "模型繁忙，请稍后重试"},
		{name: "stopped", err: errors.New("已停止生成"), want: "已停止生成"},
		{name: "internal", err: errors.New(`dial tcp 127.0.0.1:5432: connection refused`), want: "暂时无法完成回复，请稍后重试"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PublicError(tc.err)
			if got != tc.want {
				t.Fatalf("得 %q，期望 %q", got, tc.want)
			}
			if tc.err != nil && strings.Contains(got, "sk-") {
				t.Fatal("内部错误外泄")
			}
			if tc.name == "internal" && strings.Contains(got, "tcp") {
				t.Fatal("内部错误外泄")
			}
		})
	}
}
