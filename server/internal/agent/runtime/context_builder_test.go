package runtime

import (
	"errors"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	"strings"
	"testing"
	"time"
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

// Observer 检查观察分段与 Builder 的最终结果一致，空白段不触发回调。
func TestBuilderObserver(t *testing.T) {
	var observed []Segment
	var slots []string
	builder := Builder{Tools: Tools{Toolset: tool.Toolset{Entries: []tool.Entry{{Enabled: true, Spec: tool.Spec{Name: "current_time", Title: "当前时间", Available: true, Description: "查询当前时间。"}}}}}, Observer: func(slot string, segment Segment, elapsed time.Duration) {
		if elapsed < 0 {
			t.Fatal("耗时为负")
		}
		slots = append(slots, slot)
		observed = append(observed, segment)
	}}
	result, err := builder.Build(t.Context(), Input{PlatformRules: "规则", SoulMD: " ", Current: "问题", History: []provider.Message{{Role: "assistant", Content: "历史"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != len(result.Segments) {
		t.Fatal("观察数量不符", len(observed), len(result.Segments))
	}
	for i, segment := range observed {
		if segment.Name != result.Segments[i].Name || segment.EstimatedTokens != result.Segments[i].EstimatedTokens || slots[i] != segment.Name {
			t.Fatal("观察到非最终分段", segment)
		}
	}
}

func TestCurrentPreservedAndContextLimit(t *testing.T) {
	current := "  \n" + strings.Repeat("长文", 10000) + "\n  "
	result, err := (Builder{}).Build(t.Context(), Input{Current: current})
	if err != nil || len(result.Messages) != 1 || result.Messages[0].Content != current {
		t.Fatalf("当前消息未完整保留: %v", err)
	}
	if err := ValidateCurrent(strings.Repeat("字", ModelContextTokens)); err != nil {
		t.Fatal(err)
	}
	_, err = (Builder{}).Build(t.Context(), Input{Current: strings.Repeat("字", ModelContextTokens+1)})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("err=%v", err)
	}
}

func TestTruncateTokensUnicode(t *testing.T) {
	for _, text := range []string{strings.Repeat("中文🙂", 100), strings.Repeat("abcd", 100)} {
		got := TruncateTokens(text, 20)
		if EstimateTokens(got) > 20 || !strings.HasSuffix(got, "…（已截断）") {
			t.Fatal(got)
		}
	}
}
