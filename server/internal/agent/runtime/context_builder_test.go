package runtime

import (
	"linq-bs/server/internal/infra/provider"
	"testing"
)

// 使用真实 Builder 校验顺序、空白跳过和历史原文保留，不替换任何依赖。
func TestBuilderOrderAndSkip(t *testing.T) {
	result, err := (Builder{}).Build(t.Context(), Input{PlatformRules: "平台规则", SoulMD: "友好回答", History: []provider.Message{{Role: "user", Content: "第一问"}, {Role: "assistant", Content: "  保留代码缩进\n"}}, Current: "当前输入"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"platform", "soul", "history", "history", "current"}
	if len(result.Segments) != len(want) {
		t.Fatalf("segments = %+v", result.Segments)
	}
	total := 0
	for i, name := range want {
		if result.Segments[i].Name != name || result.Segments[i].EstimatedTokens < 1 {
			t.Fatalf("segment %d = %+v", i, result.Segments[i])
		}
		total += result.Segments[i].EstimatedTokens
	}
	if result.Messages[3].Content != "  保留代码缩进\n" {
		t.Fatal("历史内容被改写")
	}
	if total != result.TotalEstimatedTokens {
		t.Fatal("总估算与分段不一致")
	}
	empty, err := (Builder{}).Build(t.Context(), Input{SoulMD: " \n", Current: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Messages) != 1 || empty.Segments[0].Name != "current" {
		t.Fatalf("空模块没有跳过: %+v", empty)
	}
}
