package run

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	toolfake "linq-bs/server/internal/tool/fake"
)

// assistantFinalize 镜像 RunService.ExecuteChat 中 budget 终态与 assistant 消息字段（仅供测试）。
func assistantFinalize(outcome Outcome, visible string, execErr error) (runStatus, assistantStatus, errText, errorCode string) {
	errText = ""
	assistantStatus = "completed"
	switch outcome.Kind {
	case OutcomeBudgetExhausted:
		runStatus = "budget_exhausted"
		assistantStatus = "failed"
		errText = outcome.Message
		if errText == "" {
			errText = "已达到本轮资源上限"
		}
	case OutcomeCompleted:
		runStatus = "completed"
	default:
		runStatus = string(outcome.Kind)
	}
	if execErr == nil && visible != "" && outcome.Kind != OutcomeBudgetExhausted && outcome.Kind != OutcomeCancelled {
		runStatus = "completed"
		assistantStatus = "completed"
		errText = ""
	}
	if outcome.Code != "" {
		errorCode = outcome.Code
	} else {
		errorCode = string(outcome.Kind)
	}
	return runStatus, assistantStatus, errText, errorCode
}

func TestBudgetWallTimeExhaustedNativeEngineE2E(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_delay")
	store := NewMemStore()
	limits := definition.RuntimeLimits{
		MaxWallTimeMS: 60, MaxModelTurns: 40, MaxToolCalls: 40, MaxCallsPerStep: 8, MaxParallelTools: 4,
	}
	var script []FakeScriptEntry
	for i := 0; i < 12; i++ {
		script = append(script, ScriptToolCalls("",
			provider.ToolCall{ID: fmt.Sprintf("d%d", i), Name: "fake_delay", Arguments: `{"sleep_ms":35}`},
		))
	}
	script = append(script, ScriptResponse("不应到达"))
	rc := testRC(t, store, reg, &FakeProvider{Script: script}, limits, ts)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != OutcomeBudgetExhausted || out.Code != "wall_time" {
		t.Fatalf("期望 budget_exhausted/wall_time，得 %+v", out)
	}
	runSt, msgSt, errText, code := assistantFinalize(out, rc.VisibleText.String(), nil)
	if runSt != "budget_exhausted" || msgSt != "failed" || errText != "已达到本轮资源上限" || code != "wall_time" {
		t.Fatalf("finalize: run=%s msg=%s err=%q code=%s", runSt, msgSt, errText, code)
	}
}

func TestBudgetTokensExhaustedNativeEngineE2E(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_echo")
	store := NewMemStore()
	limits := definition.RuntimeLimits{
		MaxTotalTokens: 400, MaxOutputTokens: 512, MaxModelTurns: 4, MaxToolCalls: 4,
	}
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{ScriptResponse("x")}}, limits, ts)
	rc.Messages = []provider.Message{{Role: "user", Content: strings.Repeat("测", 800)}}
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != OutcomeBudgetExhausted || out.Code != "tokens" {
		t.Fatalf("期望 budget_exhausted/tokens，得 %+v", out)
	}
	runSt, msgSt, errText, code := assistantFinalize(out, rc.VisibleText.String(), nil)
	if runSt != "budget_exhausted" || msgSt != "failed" || errText != "已达到本轮资源上限" || code != "tokens" {
		t.Fatalf("finalize: run=%s msg=%s err=%q code=%s", runSt, msgSt, errText, code)
	}
}

func TestSchedulerTxPathSerializesParallelDelays(t *testing.T) {
	reg := tool.NewRegistry()
	_ = toolfake.Register(reg)
	ts := testToolset(t, reg, "fake_delay")
	store := NewMemStore()
	tx, err := store.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rc := testRC(t, store, reg, &FakeProvider{}, definition.RuntimeLimits{MaxParallelTools: 4, MaxToolCalls: 8}, ts)
	rc.Tx = tx
	accepted := []PlannedCall{
		{Accepted: true, Call: provider.ToolCall{ID: "p1", Name: "fake_delay", Arguments: `{"sleep_ms":60}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "p2", Name: "fake_delay", Arguments: `{"sleep_ms":60}`}},
		{Accepted: true, Call: provider.ToolCall{ID: "p3", Name: "fake_delay", Arguments: `{"sleep_ms":60}`}},
	}
	start := time.Now()
	_, err = (Scheduler{}).Run(context.Background(), rc, accepted)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	// 并行批在 rc.Tx 非空时串行执行，三路 60ms 延迟总耗时应接近串行而非单路。
	if elapsed < 140*time.Millisecond {
		t.Fatalf("Tx 路径应对同批只读工具串行执行，耗时过短: %v", elapsed)
	}
}
