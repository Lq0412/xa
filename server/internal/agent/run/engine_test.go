package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	toolfake "linq-bs/server/internal/tool/fake"
)

func testToolset(t *testing.T, reg *tool.Registry, names ...string) tool.Toolset {
	t.Helper()
	bindings := make([]binding.ToolBinding, 0, len(names))
	for _, n := range names {
		bindings = append(bindings, binding.ToolBinding{Name: n, Enabled: true})
	}
	ts, err := reg.Resolve(tool.ResolveInput{Bindings: bindings, Principal: auth.Principal{UserID: "u1"}, CanApprove: false})
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func testRC(t *testing.T, store Store, reg *tool.Registry, prov provider.ChatProvider, limits definition.RuntimeLimits, ts tool.Toolset) *RunContext {
	t.Helper()
	run := Run{ID: "run_test", ConversationID: "conv_test", Model: "test", Limits: limits, Toolset: ts}
	return &RunContext{
		Run: run, Toolset: ts, Messages: []provider.Message{{Role: "user", Content: "hi"}},
		Budget: NewBudget(limits), Store: store, Provider: prov, Registry: reg,
		Env:    tool.Env{Principal: auth.Principal{UserID: "u1"}, ConversationID: "conv_test", RunID: run.ID},
		Limits: limits, Emitter: NewV1Emitter(func(string) error { return nil }),
		CallHashes: map[string]int{}, PerToolCalls: map[string]int{}, WallStarted: time.Now(),
	}
}

func TestValidateRejectionCodes(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	limits := definition.RuntimeLimits{MaxCallsPerStep: 1, MaxToolCalls: 1, MaxModelTurns: 4}
	ts := testToolset(t, reg, "fake_echo", "fake_error")
	rc := testRC(t, NewMemStore(), reg, &FakeProvider{}, limits, ts)
	assertCode := func(t *testing.T, plan CallPlan, idx int, want string) {
		t.Helper()
		if plan.Items[idx].ErrorCode != want {
			t.Fatalf("索引 %d：期望 %s，得 %s", idx, want, plan.Items[idx].ErrorCode)
		}
	}
	plan := Validate([]provider.ToolCall{
		{ID: "c1", Name: "fake_echo", Arguments: `{}`},
		{ID: "c2", Name: "fake_echo", Arguments: `{}`},
	}, rc)
	assertCode(t, plan, 1, RejectTooManyCallsInStep)
	rc2 := testRC(t, NewMemStore(), reg, &FakeProvider{}, limits, ts)
	rc2.Budget.ToolCalls = 1
	plan = Validate([]provider.ToolCall{{ID: "c3", Name: "fake_echo", Arguments: `{}`}}, rc2)
	assertCode(t, plan, 0, RejectToolBudgetExhausted)
	plan = Validate([]provider.ToolCall{{ID: "c4", Name: "nope", Arguments: `{}`}}, rc)
	assertCode(t, plan, 0, RejectUnknownTool)
	plan = Validate([]provider.ToolCall{{ID: "c5", Name: "fake_echo", Arguments: `{`}}, rc)
	assertCode(t, plan, 0, RejectInvalidArguments)
	tsHuge := testToolset(t, reg, "fake_huge")
	rcHuge := testRC(t, NewMemStore(), reg, &FakeProvider{}, definition.RuntimeLimits{MaxToolCalls: 4}, tsHuge)
	plan = Validate([]provider.ToolCall{{ID: "c6", Name: "fake_huge", Arguments: `{"size":0}`}}, rcHuge)
	assertCode(t, plan, 0, RejectSchemaViolation)
	tsAppr, err := reg.Resolve(tool.ResolveInput{
		Bindings:   []binding.ToolBinding{{Name: "fake_approval", Enabled: true}},
		CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rcAppr := testRC(t, NewMemStore(), reg, &FakeProvider{}, definition.RuntimeLimits{MaxToolCalls: 4}, tsAppr)
	plan = Validate([]provider.ToolCall{{ID: "c7", Name: "fake_approval", Arguments: `{}`}}, rcAppr)
	assertCode(t, plan, 0, RejectToolUnavailable)
	tsCap, err := reg.Resolve(tool.ResolveInput{
		Bindings:   []binding.ToolBinding{{Name: "fake_echo", Enabled: true, MaxCalls: 1}},
		CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rcCap := testRC(t, NewMemStore(), reg, &FakeProvider{}, definition.RuntimeLimits{MaxToolCalls: 4}, tsCap)
	rcCap.PerToolCalls["fake_echo"] = 1
	plan = Validate([]provider.ToolCall{{ID: "c8", Name: "fake_echo", Arguments: `{}`}}, rcCap)
	assertCode(t, plan, 0, RejectToolCallLimit)
	rcRep := testRC(t, NewMemStore(), reg, &FakeProvider{}, definition.RuntimeLimits{MaxCallsPerStep: 8, MaxToolCalls: 8}, ts)
	plan = Validate([]provider.ToolCall{
		{ID: "r1", Name: "fake_echo", Arguments: `{"message":"same"}`},
		{ID: "r2", Name: "fake_echo", Arguments: `{"message":"same"}`},
		{ID: "r3", Name: "fake_echo", Arguments: `{"message":"same"}`},
	}, rcRep)
	assertCode(t, plan, 2, RejectRepeatedCall)
}

func TestSchedulerWriteBarrier(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	toolfake.ResetCounters()
	ts := testToolset(t, reg, "fake_read", "fake_write", "fake_echo")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxParallelTools: 4, MaxToolCalls: 8, MaxModelTurns: 2}
	rc := testRC(t, store, reg, &FakeProvider{}, limits, ts)
	accepted := []PlannedCall{
		{Accepted: true, Call: provider.ToolCall{ID: "a", Name: "fake_read", Arguments: `{}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "b", Name: "fake_write", Arguments: `{}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "c", Name: "fake_echo", Arguments: `{}`}},
	}
	if _, err := (Scheduler{}).Run(context.Background(), rc, accepted); err != nil {
		t.Fatal(err)
	}
	msgs, _ := store.ListRunMessages(context.Background(), rc.Run.ID)
	var names []string
	for _, m := range msgs {
		if m.Role == "tool" {
			names = append(names, m.ToolName)
		}
	}
	if len(names) != 3 || names[0] != "fake_read" || names[1] != "fake_write" || names[2] != "fake_echo" {
		t.Fatalf("屏障顺序错误: %v", names)
	}
}

var (
	testPeakMu sync.Mutex
	testPeak   int
	testCur    int
)

type peakSleepTool struct{}

func (peakSleepTool) Spec() tool.Spec {
	return tool.Spec{Name: "peak_sleep", Version: "1", Title: "峰值", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true, DefaultTimeout: time.Minute, MaxCallsPerRun: 16, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (peakSleepTool) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	testPeakMu.Lock()
	testCur++
	n := testCur
	if n > testPeak {
		testPeak = n
	}
	testPeakMu.Unlock()
	defer func() {
		testPeakMu.Lock()
		testCur--
		testPeakMu.Unlock()
	}()
	time.Sleep(30 * time.Millisecond)
	return tool.Result{Data: map[string]any{"ok": true}}, nil
}

func TestSchedulerBatchingAndParallelCap(t *testing.T) {
	reg := tool.NewRegistry()
	_ = reg.Register(peakSleepTool{})
	ts := testToolset(t, reg, "peak_sleep")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxParallelTools: 2, MaxToolCalls: 8, MaxModelTurns: 2}
	rc := testRC(t, store, reg, &FakeProvider{}, limits, ts)
	testPeakMu.Lock()
	testPeak, testCur = 0, 0
	testPeakMu.Unlock()
	accepted := []PlannedCall{
		{Accepted: true, Call: provider.ToolCall{ID: "1", Name: "peak_sleep", Arguments: `{}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "2", Name: "peak_sleep", Arguments: `{}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "3", Name: "peak_sleep", Arguments: `{}`}},
	}
	if _, err := (Scheduler{}).Run(context.Background(), rc, accepted); err != nil {
		t.Fatal(err)
	}
	testPeakMu.Lock()
	peak := testPeak
	testPeakMu.Unlock()
	if peak > 2 {
		t.Fatalf("并发峰值 %d 超过 2", peak)
	}
}

func TestBudgetExhaustion(t *testing.T) {
	b := NewBudget(definition.RuntimeLimits{MaxModelTurns: 2, MaxToolCalls: 1})
	b.Turns = 1
	if !b.NeedWrapUp() {
		t.Fatal("应进入收尾")
	}
}

func TestHistoryProjectorPairing(t *testing.T) {
	store := NewMemStore()
	store.SeedRun(Run{ID: "r1", ConversationID: "c1", Status: "completed", ToolCalls: 1, StartedAt: time.Now()})
	_ = store.AppendRunMessage(context.Background(), nil, RunMessage{RunID: "r1", Ordinal: 1, Role: "assistant", Content: "好的", ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "fake_echo", Arguments: `{}`}}})
	_ = store.AppendRunMessage(context.Background(), nil, RunMessage{RunID: "r1", Ordinal: 2, Role: "tool", Content: `{"ok":true}`, ToolCallID: "tc1", ToolName: "fake_echo"})
	hp := HistoryProjector{Store: store, Limits: definition.RuntimeLimits{HistoryToolRuns: 1}}
	store.LinkMessageRun("m1", "r1")
	out, err := hp.Project(context.Background(), "c1", []VisibleMessage{
		{ID: "u1", Role: "user", Content: "查"},
		{ID: "m1", Role: "assistant", Content: "好的", RunID: "r1"},
	}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range out {
		if len(m.ToolCalls) > 0 {
			if i+1 >= len(out) || out[i+1].Role != "tool" {
				t.Fatal("配对失败")
			}
		}
	}
}

func TestV1ProjectionJoinsDeltas(t *testing.T) {
	var deltas strings.Builder
	em := NewV1Emitter(func(d string) error { deltas.WriteString(d); return nil })
	em.PendingSeparator("\n\n")
	_ = em.TextDelta("第一句")
	em.PendingSeparator("\n\n")
	_ = em.TextDelta("第二句")
	if deltas.String() != "第一句\n\n第二句" {
		t.Fatal(deltas.String())
	}
	if JoinedVisible("第一句", "第二句") != "第一句\n\n第二句" {
		t.Fatal("拼接不一致")
	}
}

func TestEngineTextThenToolsFixture(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_echo")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 4, MaxParallelTools: 2}
	prov := &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("前言", provider.ToolCall{ID: "t1", Name: "fake_echo", Arguments: `{"message":"ok"}`}),
		ScriptResponse("结语"),
	}}
	rc := testRC(t, store, reg, prov, limits, ts)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil || out.Kind != OutcomeCompleted {
		t.Fatalf("%v %v", out, err)
	}
	if !strings.Contains(rc.VisibleText.String(), "前言") || !strings.Contains(rc.VisibleText.String(), "结语") {
		t.Fatal(rc.VisibleText.String())
	}
}

func TestBudgetWrapUpMaxToolCalls(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_echo")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 5, MaxToolCalls: 1, MaxParallelTools: 2}
	prov := &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("", provider.ToolCall{ID: "t1", Name: "fake_echo", Arguments: `{}`}),
		ScriptResponse("收尾回答"),
	}}
	rc := testRC(t, store, reg, prov, limits, ts)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil || out.Kind != OutcomeCompleted {
		t.Fatal(out, err)
	}
	if rc.Budget.ToolCalls > 1 {
		t.Fatal(rc.Budget.ToolCalls)
	}
}

func TestBudgetWallTimeExhausted(t *testing.T) {
	b := NewBudget(definition.RuntimeLimits{MaxWallTimeMS: 1})
	time.Sleep(2 * time.Millisecond)
	if err := b.HardCheck(5); err == nil {
		t.Fatal("应墙钟耗尽")
	}
}

func TestBudgetTokensExhausted(t *testing.T) {
	b := NewBudget(definition.RuntimeLimits{MaxTotalTokens: 100})
	b.TotalTokens = 95
	_, hard, _ := b.TokenPreCheck([]provider.Message{{Role: "user", Content: strings.Repeat("x", 400)}}, 0, 50)
	if !hard {
		t.Fatal("应 token 硬停")
	}
}

func TestToolErrorFeedbackSelfCorrect(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_error")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 4}
	prov := &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("", provider.ToolCall{ID: "e1", Name: "fake_error", Arguments: `{"code":"biz"}`}),
		ScriptToolCalls("", provider.ToolCall{ID: "e2", Name: "fake_echo", Arguments: `{"message":"fixed"}`}),
		ScriptResponse("完成"),
	}}
	rc := testRC(t, store, reg, prov, limits, ts)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil || out.Kind != OutcomeCompleted {
		t.Fatal(out, err)
	}
}

func TestToolErrorThreeFailuresWrapUp(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_error")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 8, MaxToolCalls: 12}
	script := []FakeScriptEntry{}
	for i := 0; i < 4; i++ {
		script = append(script, ScriptToolCalls("", provider.ToolCall{ID: fmt.Sprintf("e%d", i), Name: "fake_error", Arguments: `{"code":"biz"}`}))
	}
	script = append(script, ScriptResponse("收尾"))
	prov := &FakeProvider{Script: script}
	rc := testRC(t, store, reg, prov, limits, ts)
	out, _ := (&NativeEngine{}).Execute(context.Background(), rc)
	if out.Kind != OutcomeCompleted {
		t.Fatal(out)
	}
}

func TestHugeResultTruncationAndToolResultRead(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_huge")
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 2, MaxToolCalls: 2, ToolResultMaxTok: 50}
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("", provider.ToolCall{ID: "h1", Name: "fake_huge", Arguments: `{"size":8000}`}),
		ScriptResponse("done"),
	}}, limits, ts)
	_, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := store.ListRunMessages(context.Background(), rc.Run.ID)
	for _, m := range msgs {
		if m.Role == "tool" && !strings.Contains(m.Content, "result_ref") {
			t.Fatal("应截断并带 result_ref", m.Content)
		}
	}
}

func TestV1RejectsApprovalToolCall(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	toolfake.ResetCounters()
	ts, err := reg.Resolve(tool.ResolveInput{
		Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}}, CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range ts.Entries {
		if e.Spec.Name == "fake_approval" && !e.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("审批工具应在 v1 排除")
	}
	for _, mt := range ts.ModelTools() {
		if mt.Name == "fake_approval" {
			t.Fatal("ModelTools 不得含 fake_approval")
		}
	}
	plan := Validate([]provider.ToolCall{{ID: "a1", Name: "fake_approval", Arguments: `{}`}},
		testRC(t, NewMemStore(), reg, &FakeProvider{}, definition.RuntimeLimits{MaxToolCalls: 4}, ts))
	if plan.Items[0].ErrorCode != RejectToolUnavailable {
		t.Fatal(plan.Items[0].ErrorCode)
	}
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 4}
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("", provider.ToolCall{ID: "a1", Name: "fake_approval", Arguments: `{}`}),
		ScriptResponse("已拒绝审批工具"),
	}}, limits, ts)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind == OutcomeWaitingApproval {
		t.Fatal("P3 不得进入 waiting_approval")
	}
	if out.Kind != OutcomeCompleted {
		t.Fatal(out.Kind)
	}
	if toolfake.ApprovalCount() != 0 {
		t.Fatal("审批工具 Execute 不得被调用")
	}
	msgs, _ := store.ListRunMessages(context.Background(), rc.Run.ID)
	var sawTool bool
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "a1" {
			sawTool = true
			if !strings.Contains(m.Content, "client_cannot_approve") && !strings.Contains(m.Content, "不可用") {
				t.Fatal(m.Content)
			}
		}
	}
	if !sawTool {
		t.Fatal("应写入拒绝 tool 消息")
	}
}

func TestMemStoreRoundtrip(t *testing.T) {
	store := NewMemStore()
	run := Run{ID: "run_rt", ConversationID: "c1", Status: "running", StartedAt: time.Now()}
	_ = store.InsertRun(context.Background(), nil, run)
	snap := RunContextSnapshot{RunID: run.ID, Messages: []provider.Message{{Role: "user", Content: "x"}}, Segments: json.RawMessage(`[]`)}
	_ = store.SaveRunContext(context.Background(), nil, snap)
	got, err := store.GetRun(context.Background(), run.ID)
	if err != nil || got.ID != run.ID {
		t.Fatal(err)
	}
	loaded, err := store.LoadRunContext(context.Background(), run.ID)
	if err != nil || len(loaded.Messages) != 1 {
		t.Fatal(err)
	}
}
