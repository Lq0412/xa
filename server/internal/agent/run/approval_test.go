package run

import (
	"context"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
	toolfake "linq-bs/server/internal/tool/fake"
)

// 等待审批不能写助手终态消息，也不能发 run.completed。
func TestApprovalWaitingDoesNotFinalize(t *testing.T) {
	store := NewMemStore()
	store.SeedRun(Run{ID: "waiting", Status: "waiting_approval"})
	bus := NewEventBus("waiting", nil, nil, store)
	rc := &RunContext{Run: Run{ID: "waiting"}, Store: store, Budget: NewBudget(definition.RuntimeLimits{}), Bus: bus, Emitter: &CombinedSink{Bus: bus}}
	s := &RunService{Store: store}
	if err := s.finalizeStreamingRun(context.Background(), rc, Outcome{Kind: OutcomeWaitingApproval}, nil, 0, StreamStartInput{RunID: "waiting"}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRun(context.Background(), "waiting")
	if got.Status != "waiting_approval" || got.FinalMessageID != "" {
		t.Fatalf("等待被错误收尾 %#v", got)
	}
	events, _ := store.ListRunEventsAfter(context.Background(), "waiting", 0)
	if len(events) != 0 {
		t.Fatalf("等待不能发终止事件 %v", events)
	}
}

func TestApprovalResumeDecisionAndHash(t *testing.T) {
	for _, decision := range []string{"approved", "rejected", "expired", "mismatch"} {
		t.Run(decision, func(t *testing.T) {
			reg := tool.NewRegistry()
			if err := toolfake.Register(reg); err != nil {
				t.Fatal(err)
			}
			toolfake.ResetCounters()
			ts, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}, {Name: "fake_echo", Enabled: true}}, CanApprove: true})
			if err != nil {
				t.Fatal(err)
			}
			store := NewMemStore()
			limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 8, MaxWallTimeMS: 10000}
			prov := &FakeProvider{Script: []FakeScriptEntry{ScriptToolCalls("前言", provider.ToolCall{ID: "a", Name: "fake_approval", Arguments: `{}`}, provider.ToolCall{ID: "b", Name: "fake_echo", Arguments: `{}`}), ScriptResponse("结语")}}
			rc := testRC(t, store, reg, prov, limits, ts)
			rc.Run.Status, rc.Run.StreamProtocol = "running", 2
			store.SeedRun(rc.Run)
			out, err := (&NativeEngine{}).Execute(t.Context(), rc)
			if err != nil || out.Kind != OutcomeWaitingApproval {
				t.Fatal(out, err)
			}
			for id, a := range store.approvals {
				a.Status = decision
				if decision == "mismatch" {
					a.Status = "approved"
					a.ArgsHash = "changed"
				}
				store.approvals[id] = a
			}
			// 用新 RunContext 恢复，原来的进程状态完全丢弃。
			svc := &RunService{Store: store, Registry: reg, Provider: prov}
			restored, err := svc.restoreContext(t.Context(), rc.Run.ID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err = (&NativeEngine{}).Execute(t.Context(), restored)
			if err != nil || out.Kind != OutcomeCompleted {
				t.Fatal(out, err)
			}
			wantCount := int64(0)
			if decision == "approved" {
				wantCount = 1
			}
			if toolfake.ApprovalCount() != wantCount {
				t.Fatal("错误副作用次数", toolfake.ApprovalCount())
			}
			row, err := store.GetToolResultByCallID(t.Context(), rc.Run.ID, "a")
			if err != nil {
				t.Fatal(err)
			}
			codes := map[string]string{"rejected": "approval_rejected", "expired": "approval_expired", "mismatch": "args_mismatch"}
			if row.ErrorCode != codes[decision] {
				t.Fatal(row.ErrorCode)
			}
			if !strings.Contains(restored.VisibleText.String(), "前言") || !strings.Contains(restored.VisibleText.String(), "结语") {
				t.Fatal(restored.VisibleText.String())
			}
			later, err := store.GetToolResultByCallID(t.Context(), rc.Run.ID, "b")
			if err != nil || later.Status != "succeeded" {
				t.Fatal("未恢复后续批次", later, err)
			}
			if restored.Budget.Turns != 2 || restored.Budget.ToolCalls != 2 {
				t.Fatal("预算恢复错误", restored.Budget)
			}
		})
	}
}

func TestConversationGrantDoesNotChangeV1Toolset(t *testing.T) {
	reg := tool.NewRegistry()
	if err := toolfake.Register(reg); err != nil {
		t.Fatal(err)
	}
	for _, canApprove := range []bool{true, false} {
		ts, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true, Approval: "never"}}, CanApprove: canApprove})
		if err != nil {
			t.Fatal(err)
		}
		for _, mt := range ts.ModelTools() {
			if mt.Name == "fake_approval" && !canApprove {
				t.Fatal("Agent 授予不能绕过 v1 能力")
			}
		}
	}

	ts, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}}, CanApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemStore()
	store.grants["conv_test/u1/fake_approval"] = true
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{ScriptToolCalls("", provider.ToolCall{ID: "a", Name: "fake_approval", Arguments: `{}`}), ScriptResponse("完成")}}, definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 8}, ts)
	out, err := (&NativeEngine{}).Execute(t.Context(), rc)
	if err != nil || out.Kind != OutcomeCompleted || len(store.approvals) != 0 {
		t.Fatal("对话授予未生效", out, err)
	}
}

// 同步模型脚本仅用于引擎回归，审批前后使用真实调度和存储实现。
func TestApprovalSuspendsBeforeLaterBatch(t *testing.T) {
	reg := tool.NewRegistry()
	if err := toolfake.Register(reg); err != nil {
		t.Fatal(err)
	}
	ts, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}, {Name: "fake_echo", Enabled: true}}, CanApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemStore()
	limits := definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 8}
	rc := testRC(t, store, reg, &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("先确认", provider.ToolCall{ID: "approve", Name: "fake_approval", Arguments: `{}`}, provider.ToolCall{ID: "later", Name: "fake_echo", Arguments: `{}`}),
		ScriptResponse("已完成"),
	}}, limits, ts)
	rc.Run.Status = "running"
	rc.Run.StreamProtocol = 2
	store.SeedRun(rc.Run)
	out, err := (&NativeEngine{}).Execute(context.Background(), rc)
	if err != nil || out.Kind != OutcomeWaitingApproval {
		t.Fatalf("应挂起，得到 %v %v", out, err)
	}
	pending, err := store.PendingCallsOfLastStep(context.Background(), rc.Run.ID)
	if err != nil || len(pending) != 2 {
		t.Fatalf("未持久化剩余调用 %v %v", pending, err)
	}
	msgs, _ := store.ListRunMessages(context.Background(), rc.Run.ID)
	for _, m := range msgs {
		if m.Role == "tool" {
			t.Fatalf("审批前执行了工具 %#v", m)
		}
	}
}

type fixedApprovalTool struct{ toolfake.NeedsApproval }

func (f fixedApprovalTool) Spec() tool.Spec {
	spec := f.NeedsApproval.Spec()
	spec.Approval.Overridable = false
	return spec
}

// 强制审批工具可以只允许本次，Agent 配置不能取消后续确认。
func TestFixedApprovalCannotChangeAgentPolicy(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(fixedApprovalTool{}); err != nil {
		t.Fatal(err)
	}
	toolset, err := reg.Resolve(tool.ResolveInput{Bindings: []binding.ToolBinding{{Name: "fake_approval", Enabled: true}}, CanApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	toolfake.ResetCounters()
	store := NewMemStore()
	provider := &FakeProvider{Script: []FakeScriptEntry{
		ScriptToolCalls("", provider.ToolCall{ID: "one", Name: "fake_approval", Arguments: `{}`}),
		ScriptResponse("完成"),
	}}
	rc := testRC(t, store, reg, provider, definition.RuntimeLimits{MaxModelTurns: 4, MaxToolCalls: 4}, toolset)
	rc.CanManageAgent = true
	store.SeedRun(rc.Run)
	engine := &NativeEngine{}
	out, err := engine.Execute(t.Context(), rc)
	if err != nil || out.Kind != OutcomeWaitingApproval {
		t.Fatal(out, err)
	}
	svc := &RunService{Store: store, Registry: reg, Provider: provider}
	for id, approval := range store.approvals {
		if approval.CanRememberAgent || svc.rememberAgent(t.Context(), rc.Env.Principal, rc.Run.AgentID, approval.Name) == nil {
			t.Fatal("强制审批的工具可以取消后续确认")
		}
		approval.Status = "approved"
		store.approvals[id] = approval
	}
	restored, err := svc.restoreContext(t.Context(), rc.Run.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err = engine.Execute(t.Context(), restored)
	if err != nil || out.Kind != OutcomeCompleted || toolfake.ApprovalCount() != 1 {
		t.Fatal("本次批准未执行", out, err)
	}
}
