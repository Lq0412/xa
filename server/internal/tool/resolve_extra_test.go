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

func TestResolveRolesAndEntry(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(stubTool{spec: Spec{
		Name: "role_tool", Version: "1", DefaultEnabled: true, Available: true,
		InputSchema: json.RawMessage(`{"type":"object"}`), DefaultTimeout: time.Second, MaxCallsPerRun: 4,
		Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true},
	}})
	ts, err := r.Resolve(ResolveInput{
		Bindings:     []binding.ToolBinding{{Name: "role_tool", Enabled: true, AllowedRoles: []string{"admin"}}},
		MemoryPolicy: definition.MemoryPolicy{Enabled: true}, Principal: auth.Principal{Roles: []string{"user"}}, CanApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	e, ok := ts.EntryByName("role_tool")
	if !ok || e.Reason != "role_denied" {
		t.Fatalf("%+v", e)
	}
	if !rolesIntersect([]string{"admin", "user"}, []string{"user"}) {
		t.Fatal()
	}
	_, ok = Toolset{}.EntryByName("missing")
	if ok {
		t.Fatal()
	}
}

func TestRegistryRegisterNil(t *testing.T) {
	if err := NewRegistry().Register(nil); err == nil {
		t.Fatal()
	}
	if err := NewRegistry().Register(stubTool{spec: Spec{Name: "BAD-NAME"}}); err == nil {
		t.Fatal()
	}
}

func TestToolErrorString(t *testing.T) {
	e := &Error{Message: "x"}
	if e.Error() != "x" {
		t.Fatal()
	}
	_, _ = stubTool{}.Execute(context.Background(), Call{}, Env{})
}
