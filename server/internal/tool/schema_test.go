package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaSubsetDocument(t *testing.T) {
	for name, raw := range map[string]string{
		"ok":     `{"type":"object","properties":{"q":{"type":"string","maxLength":10}}}`,
		"bad_kw": `{"type":"object","$ref":"#/defs/x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateSchemaDocument(json.RawMessage(raw))
			if name == "ok" && err != nil {
				t.Fatal(err)
			}
			if name != "ok" && err == nil {
				t.Fatal("应拒绝未知关键字")
			}
		})
	}
}

func TestSchemaValidateInstance(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1,"maxLength":5},"top_k":{"type":"integer","minimum":1,"maximum":10}}}`)
	violations := ValidateJSON(schema, json.RawMessage(`{"query":"hello","top_k":11}`))
	if len(violations) == 0 || !strings.Contains(violations[0].Error(), "必须") {
		t.Fatalf("期望 top_k 违规: %v", violations)
	}
	if ok := ValidateJSON(schema, json.RawMessage(`{"query":"ok"}`)); len(ok) != 0 {
		t.Fatal(ok)
	}
}
