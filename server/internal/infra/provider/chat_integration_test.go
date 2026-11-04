//go:build integration

package provider

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestBailianStream 使用 .env 注入的真实凭证验证流式文本和 token 用量。
func TestBailianStream(t *testing.T) {
	key := os.Getenv("XA_BAILIAN_API_KEY")
	base := os.Getenv("XA_BAILIAN_BASE_URL")
	if key == "" || base == "" {
		t.Fatal("缺少 XA_BAILIAN_API_KEY 或 XA_BAILIAN_BASE_URL")
	}
	var text strings.Builder
	usage, err := NewBailian(key, base).Stream(context.Background(), "qwen3.8-flash", []Message{{Role: "user", Content: "请只回复一个字：好"}}, func(delta string) error { text.WriteString(delta); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(text.String()) == "" || usage.CompletionTokens == 0 {
		t.Fatalf("流式结果或 token 用量为空: text=%q usage=%+v", text.String(), usage)
	}
}
