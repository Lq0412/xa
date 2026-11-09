package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"linq-bs/server/internal/tool"
)

func TestCurrentTimeExecute(t *testing.T) {
	ct := CurrentTime{}
	res, err := ct.Execute(context.Background(), tool.Call{}, tool.Env{Config: json.RawMessage(`{"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := res.Data.(map[string]any)
	if !ok || data["timezone"] != "UTC" {
		t.Fatal(res.Data)
	}
}
