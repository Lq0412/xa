package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/binding"
)

// TestBindingRestrictions 直接验证注册描述与绑定的约束，不提供虚构工具执行器。
func TestBindingRestrictions(t *testing.T) {
	spec := Spec{MaxCallsPerRun: 4, Approval: ApprovalPolicy{Default: ApprovalAlways}, ConfigSchema: json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string"}}}`)}
	for _, tc := range []struct {
		name    string
		binding binding.ToolBinding
		field   string
	}{
		{"max", binding.ToolBinding{MaxCalls: 100000}, "max_calls"},
		{"negative", binding.ToolBinding{MaxCalls: -1}, "max_calls"},
		{"approval", binding.ToolBinding{Approval: "never"}, "approval"},
		{"role", binding.ToolBinding{AllowedRoles: []string{"root"}}, "allowed_roles"},
		{"base64", binding.ToolBinding{Configuration: json.RawMessage(`"e30="`)}, "configuration"},
		{"array", binding.ToolBinding{Configuration: json.RawMessage(`[]`)}, "configuration"},
		{"null", binding.ToolBinding{Configuration: json.RawMessage(`null`)}, "configuration"},
		{"unknown", binding.ToolBinding{Configuration: json.RawMessage(`{"undeclared":true}`)}, "configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateBinding(tc.binding, spec); err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatal("未拒绝无效字段")
			}
		})
	}
	for _, b := range []binding.ToolBinding{{}, {MaxCalls: 4, Approval: "always", AllowedRoles: []string{"admin", "user"}, Configuration: json.RawMessage(`{"timezone":"Asia/Shanghai"}`)}} {
		if err := validateBinding(b, spec); err != nil {
			t.Fatal(err)
		}
	}
	spec.Approval.Overridable = true
	if err := validateBinding(binding.ToolBinding{Approval: "never"}, spec); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	for _, list := range [][]binding.ToolBinding{nil, {}} {
		if err := r.ValidateBindings(list); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []binding.ToolBinding{{}, {Name: "missing"}, {ToolVersionID: "legacy"}, {Name: "invalid name"}} {
		if err := r.ValidateBindings([]binding.ToolBinding{b}); err == nil {
			t.Fatal("未知工具被接受")
		}
	}
}
