package tool

import (
	"encoding/json"
	"testing"
)

func TestSchemaStringIntegerBoolean(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"flag":{"type":"boolean"},"count":{"type":"integer"},"code":{"type":"string","pattern":"^[a-z]+$","minLength":2}}}`)
	if v := ValidateJSON(schema, json.RawMessage(`{"flag":true,"count":3,"code":"ab"}`)); len(v) != 0 {
		t.Fatal(v)
	}
	if v := ValidateJSON(schema, json.RawMessage(`{"flag":"no","count":1.5,"code":"A"}`)); len(v) < 2 {
		t.Fatal(v)
	}
	if err := ValidateSchemaDocument(json.RawMessage(`{"type":"object","pattern":"["}`)); err == nil {
		t.Fatal("非法 pattern 应失败")
	}
}

func TestRequirementUnknown(t *testing.T) {
	if !requirementMet(Requirement("unknown"), false, false) {
		t.Fatal("未知 requirement 默认放行")
	}
}
