package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"linq-bs/server/internal/agent/binding"
)

type fixedTool struct{ name string }

func (f fixedTool) Spec() Spec {
	name := f.name
	if name == "" {
		name = "fixed_tool"
	}
	return Spec{
		Name: name, Version: "1", InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
		ConfigSchema:   json.RawMessage(`{"type":"object","properties":{"y":{"type":"integer","minimum":1}}}`),
		DefaultTimeout: time.Second, MaxCallsPerRun: 2, Available: true,
		Approval: ApprovalPolicy{Default: ApprovalNever, Overridable: true},
	}
}
func (fixedTool) Execute(context.Context, Call, Env) (Result, error) { return Result{}, nil }

func TestRegistryRegisterAndCatalog(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fixedTool{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(fixedTool{}); err == nil {
		t.Fatal("重复注册应失败")
	}
	if _, ok := r.Get("fixed_tool"); !ok {
		t.Fatal()
	}
	if len(r.Catalog()) != 1 {
		t.Fatal()
	}
	if err := r.ValidateBindings([]binding.ToolBinding{{Name: "fixed_tool", Enabled: true, Configuration: json.RawMessage(`{"y":2}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateBindings([]binding.ToolBinding{{Name: "fixed_tool", Configuration: json.RawMessage(`{"y":0}`)}}); err == nil {
		t.Fatal("应拒绝 schema 违规 configuration")
	}
}

func TestRegisterRejectsInvalidSchema(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(badSchemaTool{}); err == nil {
		t.Fatal("应拒绝 schema 子集外关键字")
	}
}

type badSchemaTool struct{}

func (badSchemaTool) Spec() Spec {
	s := fixedTool{name: "bad_schema_tool"}.Spec()
	s.InputSchema = json.RawMessage(`{"notAllowed":true}`)
	return s
}
func (badSchemaTool) Execute(context.Context, Call, Env) (Result, error) { return Result{}, nil }

func TestToolsetModelTools(t *testing.T) {
	ts := Toolset{Entries: []Entry{{Spec: Spec{Name: "a", Description: "d", InputSchema: json.RawMessage(`{}`), Available: true}, Enabled: true}}}
	if len(ts.ModelTools()) != 1 {
		t.Fatal()
	}
	ts.Entries[0].Enabled = false
	if len(ts.ModelTools()) != 0 {
		t.Fatal()
	}
}
