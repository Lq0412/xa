package tool

import (
	"encoding/json"
	"testing"
)

func TestSchemaInstanceBranches(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["n"],"properties":{"n":{"type":"number","minimum":1,"maximum":3},"tags":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"string"}},"mode":{"type":"string","enum":["a","b"]},"fixed":{"const":1},"nullable":{"type":["string","null"]}}}`)
	cases := []struct {
		raw    string
		wantOK bool
	}{
		{`{"n":2,"tags":["x"],"mode":"a","fixed":1,"nullable":null}`, true},
		{`{"n":9}`, false},
		{`{"extra":1,"n":1}`, false},
		{`{"n":1,"tags":[]}`, false},
		{`{"n":1,"mode":"z","fixed":1}`, false},
	}
	for _, tc := range cases {
		v := ValidateJSON(schema, json.RawMessage(tc.raw))
		if tc.wantOK && len(v) != 0 {
			t.Fatalf("%s: %v", tc.raw, v)
		}
		if !tc.wantOK && len(v) == 0 {
			t.Fatalf("应失败: %s", tc.raw)
		}
	}
	var v Violation
	_ = v.Error()
}

func TestSchemaDocumentTypes(t *testing.T) {
	if err := ValidateSchemaDocument(json.RawMessage(`{"type":["string","null"],"items":{"type":"integer"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchemaDocument(json.RawMessage(`{"type":"object","additionalProperties":false}`)); err != nil {
		t.Fatal(err)
	}
}
