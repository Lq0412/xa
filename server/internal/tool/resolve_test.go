package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
)

type stubTool struct{ spec Spec }

func (s stubTool) Spec() Spec                                       { return s.spec }
func (stubTool) Execute(context.Context, Call, Env) (Result, error) { return Result{}, nil }

func TestResolveDefaultsAndMemory(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(stubTool{spec: Spec{
		Name: "memory_search", Version: "1", DefaultEnabled: true, Available: true,
		Requires: []Requirement{RequireMemoryEnabled}, DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 8,
		Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	_ = r.Register(stubTool{spec: Spec{
		Name: "current_time", Version: "1", DefaultEnabled: true, Available: true,
		DefaultTimeout: 3 * time.Second, MaxCallsPerRun: 32,
		Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	_ = r.Register(stubTool{spec: Spec{
		Name: "memory_forget", Version: "1", DefaultEnabled: true, Available: true, Effect: EffectDestructive,
		Requires: []Requirement{RequireMemoryEnabled}, DefaultTimeout: 10 * time.Second, MaxCallsPerRun: 4,
		Approval: ApprovalPolicy{Default: ApprovalAlways, Overridable: true}, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	off, err := r.Resolve(ResolveInput{MemoryPolicy: definition.MemoryPolicy{Enabled: false}, Principal: auth.Principal{Roles: []string{"user"}}, CanApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range off.Entries {
		if e.Spec.Name == "memory_search" && (e.Enabled || e.Reason != "memory_disabled") {
			t.Fatal("记忆关闭应不可用")
		}
	}
	v1, err := r.Resolve(ResolveInput{
		Bindings:     []binding.ToolBinding{{Name: "memory_forget", Enabled: true}},
		MemoryPolicy: definition.MemoryPolicy{Enabled: true}, Principal: auth.Principal{Roles: []string{"user"}}, CanApprove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range v1.Entries {
		if e.Spec.Name == "memory_forget" && e.Reason != "client_cannot_approve" {
			t.Fatalf("v1 应排除需审批工具: %+v", e)
		}
	}
	v2, err := r.Resolve(ResolveInput{
		Bindings:     []binding.ToolBinding{{Name: "current_time", Enabled: true, Configuration: json.RawMessage(`{"timezone":"UTC"}`)}},
		MemoryPolicy: definition.MemoryPolicy{Enabled: true}, Principal: auth.Principal{Roles: []string{"user"}}, CanApprove: true,
	})
	if err != nil || v2.Hash == "" {
		t.Fatal(v2, err)
	}
}
