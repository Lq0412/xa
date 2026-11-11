package run

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	toolfake "linq-bs/server/internal/tool/fake"
)

func TestTracePreviewAndNestedAttributes(t *testing.T) {
	secret := "sk-" + strings.Repeat("x", 40)
	s := RedactSpan(Span{InputPreview: strings.Repeat("中", 600), OutputPreview: secret + " Bearer credential test@example.com 13812345678", Attributes: map[string]any{"nested": []any{map[string]any{"api_key": "opaque", "value": secret}}}})
	if len([]rune(s.InputPreview)) != 512 || !strings.HasSuffix(s.InputPreview, "…") {
		t.Fatal("预览上限错误")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, "credential", "test@example.com", "13812345678", "opaque"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("追踪包含未脱敏字段")
		}
	}
}

func TestTraceEngineTreeUsageAndRejectedTool(t *testing.T) {
	store := NewMemStore()
	reg := tool.NewRegistry()
	if err := toolfake.Register(reg); err != nil {
		t.Fatal(err)
	}
	first := ScriptToolCalls("", provider.ToolCall{ID: "ok", Name: "fake_echo", Arguments: `{"message":"hello"}`}, provider.ToolCall{ID: "bad", Name: "not_registered", Arguments: `{}`})
	first.Response.Usage = provider.Usage{PromptTokens: 12, CompletionTokens: 8, CachedTokens: 3}
	first.Response.TTFT = 7 * time.Millisecond
	second := ScriptResponse("完成")
	second.Response.Usage = provider.Usage{PromptTokens: 20, CompletionTokens: 2, CachedTokens: 4}
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{first, second}}, definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 8, MaxCallsPerStep: 4, MaxWallTimeMS: 10000}, testToolset(t, reg, "fake_echo"))
	rc.Run.StartedAt = time.Now()
	rc.initTrace(ChatStartInput{ContextStartedAt: rc.Run.StartedAt, ContextTrace: []ContextObservation{{Slot: "current", Segment: runtime.Segment{Name: "current", EstimatedTokens: 3}, StartedAt: rc.Run.StartedAt, EndedAt: time.Now()}}})
	out, err := (&NativeEngine{}).Execute(t.Context(), rc)
	if err != nil || out.Kind != OutcomeCompleted {
		t.Fatal(out, err)
	}
	rc.Run.Status = "completed"
	rc.Tracer.Finish(rc.Run)
	if err := rc.Tracer.FlushStep(t.Context()); err != nil {
		t.Fatal(err)
	}
	spans, err := store.ListSpans(t.Context(), rc.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	tokens := [3]int{}
	foundRejected := false
	for _, s := range spans {
		counts[s.Kind]++
		if s.Kind != "run" && s.ParentSpanID == "" {
			t.Fatal("缺少父节点")
		}
		if s.Kind == "llm" {
			tokens[0] += *s.PromptTokens
			tokens[1] += *s.CompletionTokens
			tokens[2] += *s.CachedTokens
		}
		if s.CallID == "bad" {
			foundRejected = s.Status == "error" && s.ErrorCode == RejectUnknownTool
		}
		if s.Kind == "run" && (*s.PromptTokens != rc.Run.PromptTokens || *s.CompletionTokens != rc.Run.CompletionTokens || *s.CachedTokens != rc.Run.CachedTokens) {
			t.Fatal("根token不一致")
		}
	}
	if counts["run"] != 1 || counts["context"] != 1 || counts["context_segment"] != 1 || counts["llm"] != 2 || counts["tool"] != 2 || !foundRejected {
		t.Fatal("执行树不完整", counts)
	}
	if tokens != [3]int{rc.Run.PromptTokens, rc.Run.CompletionTokens, rc.Run.CachedTokens} || rc.Run.TTFTMS == nil || *rc.Run.TTFTMS != 7 {
		t.Fatal("usage或TTFT不一致")
	}
}

func TestTraceConcurrentFlushAndRestore(t *testing.T) {
	store := NewMemStore()
	r := Run{ID: "trace", StartedAt: time.Now()}
	tr := NewTracer(r, PGSpanSink{Store: store}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := tr.Start(tr.RootID, "tool", "execute_tool", 1, "", time.Now())
			tr.End(s, "ok", "", time.Now())
			if err := tr.FlushStep(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	spans, err := store.ListSpans(t.Context(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := NewTracer(r, PGSpanSink{Store: store}, spans)
	if restored.RootID != tr.RootID || len(restored.Spans()) != 13 {
		t.Fatal("恢复重复创建节点")
	}
	for _, s := range spans {
		if s.Kind == "tool" && s.EndedAt == nil {
			t.Fatal("并行刷写覆盖已结束节点")
		}
	}
}
