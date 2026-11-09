package tool

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
)

// TestV1ToolsetOmitsApprovalTools v1（CanApprove=false）不得把需审批工具放进 ModelTools。
func TestV1ToolsetOmitsApprovalTools(t *testing.T) {
	r := NewRegistry()
	stubs := []stubTool{
		{spec: Spec{
			Name: "memory_search", Version: "1", DefaultEnabled: true, Available: true,
			Requires: []Requirement{RequireMemoryEnabled}, DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 8,
			Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		{spec: Spec{
			Name: "memory_write", Version: "1", DefaultEnabled: true, Available: true, Effect: EffectWrite,
			Requires: []Requirement{RequireMemoryEnabled}, DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 8,
			Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		{spec: Spec{
			Name: "current_time", Version: "1", DefaultEnabled: true, Available: true,
			DefaultTimeout: 3 * time.Second, MaxCallsPerRun: 32,
			Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		{spec: Spec{
			Name: "memory_forget", Version: "1", DefaultEnabled: true, Available: true, Effect: EffectDestructive,
			Requires: []Requirement{RequireMemoryEnabled}, DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 4,
			Approval: ApprovalPolicy{Default: ApprovalAlways, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
	for _, s := range stubs {
		if err := r.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.Register(stubTool{spec: Spec{
		Name: "fake_approval", Version: "1", DefaultEnabled: false, Available: true, Effect: EffectWrite,
		DefaultTimeout: time.Second, MaxCallsPerRun: 8,
		Approval: ApprovalPolicy{Default: ApprovalAlways, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})

	ts, err := r.Resolve(ResolveInput{
		Bindings: []binding.ToolBinding{
			{Name: "memory_forget", Enabled: true},
			{Name: "fake_approval", Enabled: true},
			{Name: "current_time", Enabled: true},
			{Name: "memory_search", Enabled: true},
			{Name: "memory_write", Enabled: true},
		},
		MemoryPolicy: definition.MemoryPolicy{Enabled: true},
		Principal:    auth.Principal{Roles: []string{"user"}},
		CanApprove:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"memory_forget", "fake_approval"} {
		e, ok := ts.EntryByName(name)
		if !ok {
			t.Fatalf("缺少工具条目: %s", name)
		}
		if e.Enabled || e.Reason != "client_cannot_approve" {
			t.Fatalf("%s 应被 v1 排除: enabled=%v reason=%q", name, e.Enabled, e.Reason)
		}
	}
	names := []string{}
	for _, mt := range ts.ModelTools() {
		names = append(names, mt.Name)
	}
	for _, blocked := range []string{"memory_forget", "fake_approval"} {
		if slices.Contains(names, blocked) {
			t.Fatalf("ModelTools 含需审批工具 %s: %v", blocked, names)
		}
	}
	for _, allowed := range []string{"current_time", "memory_search", "memory_write"} {
		if !slices.Contains(names, allowed) {
			t.Fatalf("ModelTools 缺少可用工具 %s: %v", allowed, names)
		}
	}
}

// TestV1ResolveExcludesApprovalFromRunnableSet CanApprove=false 时，需审批工具不得出现在可运行集合（ModelTools）；Validate/Execute 拒绝路径在 P3 引擎落地后验收。
func TestV1ResolveExcludesApprovalFromRunnableSet(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(stubTool{spec: Spec{
		Name: "fake_approval", Version: "1", DefaultEnabled: false, Available: true, Effect: EffectWrite,
		DefaultTimeout: time.Second, MaxCallsPerRun: 8,
		Approval: ApprovalPolicy{Default: ApprovalAlways, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	ts, err := r.Resolve(ResolveInput{
		Bindings:     []binding.ToolBinding{{Name: "fake_approval", Enabled: true}},
		MemoryPolicy: definition.MemoryPolicy{Enabled: true},
		Principal:    auth.Principal{Roles: []string{"user"}},
		CanApprove:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	runnable := map[string]bool{}
	for _, mt := range ts.ModelTools() {
		runnable[mt.Name] = true
	}
	for _, e := range ts.Entries {
		if e.Reason == "client_cannot_approve" {
			if e.Enabled || runnable[e.Spec.Name] {
				t.Fatalf("需审批工具 %s 不应可运行: enabled=%v in_model=%v", e.Spec.Name, e.Enabled, runnable[e.Spec.Name])
			}
		}
	}
}
