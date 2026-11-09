package memory

import (
	"encoding/json"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/runtime"
)

// 使用真实窗口算法检查轮次顺序、预算和持久化元数据，不替换外部依赖。
func TestSelectHistory(t *testing.T) {
	for _, tc := range []struct {
		name            string
		messages        []HistoryMessage
		budget, dropped int
		truncated       bool
		first           string
	}{
		{"empty", nil, 100, 0, false, ""},
		{"fits", []HistoryMessage{{"u1", "user", "你好", ""}, {"a1", "assistant", "你好", ""}}, 100, 0, false, "u1"},
		{"oldest_round", []HistoryMessage{{"u1", "user", strings.Repeat("旧", 70), ""}, {"a1", "assistant", "答", ""}, {"u2", "user", strings.Repeat("新", 60), ""}, {"a2", "assistant", "答", ""}}, 100, 1, false, "u2"},
		{"single_message", []HistoryMessage{{"u1", "user", strings.Repeat("长", 300), ""}}, 100, 0, true, "u1"},
		{"last_round", []HistoryMessage{{"u1", "user", "旧", ""}, {"a1", "assistant", "答", ""}, {"u2", "user", strings.Repeat("长", 300), ""}, {"a2", "assistant", "回复", ""}}, 100, 1, true, "u2"},
		{"two_long_messages", []HistoryMessage{{"u1", "user", strings.Repeat("长", 300), ""}, {"a1", "assistant", strings.Repeat("答", 200), ""}}, 100, 0, true, "u1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.messages)
			if err != nil {
				t.Fatal(err)
			}
			w := SelectHistory(tc.messages, tc.budget)
			if w.Details.DroppedRounds != tc.dropped || w.Details.Truncated != tc.truncated || w.Details.FromMessageID != tc.first {
				t.Fatalf("details=%+v", w.Details)
			}
			total := 0
			for _, m := range w.Messages {
				total += runtime.EstimateTokens(m.Content)
			}
			if total > tc.budget {
				t.Fatalf("tokens=%d", total)
			}
			if tc.truncated && (!strings.HasSuffix(w.Messages[0].Content, "…（已截断）") || len(w.Details.TruncatedMessageIDs) == 0) {
				t.Fatal("截断标记或消息 ID 缺失")
			}
			if tc.truncated || tc.dropped > 0 {
				raw, err := json.Marshal(w.Details)
				if err != nil || !strings.Contains(string(raw), `"over_budget":true`) {
					t.Fatalf("标记未序列化: %s %v", raw, err)
				}
			}
			after, err := json.Marshal(tc.messages)
			if err != nil || string(before) != string(after) {
				t.Fatal("原始数据被修改")
			}
		})
	}
}
