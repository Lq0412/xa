package conversation

import (
	"testing"
	"time"

	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
)

// 准备阶段在 Builder 之后追加 summary/history 元数据时，追踪必须使用最终分段且保留各自计时。
func TestContextObservationsFinalMetadata(t *testing.T) {
	started := time.Now().Add(-time.Second)
	ended := started.Add(12 * time.Millisecond)
	measured := []run.ContextObservation{{Slot: "session_summary", Segment: runtime.Segment{Name: "session_summary"}, StartedAt: started, EndedAt: ended}, {Slot: "history", Segment: runtime.Segment{Name: "history"}, StartedAt: ended, EndedAt: ended.Add(time.Millisecond)}, {Slot: "history", Segment: runtime.Segment{Name: "history"}, StartedAt: ended.Add(time.Millisecond), EndedAt: ended.Add(2 * time.Millisecond)}}
	segments := []runtime.Segment{{Name: "session_summary", SummaryID: "summary", SummaryVersion: 2}, {Name: "history", FromMessageID: "first", UntilMessageID: "last"}, {Name: "history", FromMessageID: "first", UntilMessageID: "last"}, {Name: "tool_schemas", ItemIDs: []string{"current_time"}}}
	observations := contextObservations(segments, measured)
	if len(observations) != len(segments) {
		t.Fatal("分段观察数量不符")
	}
	if observations[0].Segment.SummaryID != "summary" || observations[0].Segment.SummaryVersion != 2 || !observations[0].StartedAt.Equal(started) || !observations[0].EndedAt.Equal(ended) {
		t.Fatal("最终summary元数据或计时丢失")
	}
	for i := 1; i <= 2; i++ {
		if observations[i].Segment.FromMessageID != "first" || observations[i].Segment.UntilMessageID != "last" || !observations[i].StartedAt.Equal(measured[i].StartedAt) {
			t.Fatal("重复history分段计时或范围错配")
		}
	}
	if observations[3].Slot != "tool_schemas" || observations[3].StartedAt.IsZero() || observations[3].EndedAt.Before(observations[3].StartedAt) {
		t.Fatal("追加段缺少有效计时")
	}
}
