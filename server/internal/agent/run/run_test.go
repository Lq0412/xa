package run

import (
	"context"
	"testing"
)

var _ func(Engine, context.Context, *RunContext) (Outcome, error) = Engine.Execute

func TestEngineContract(t *testing.T) {
	var outcome Outcome
	if outcome.Kind != "" || outcome.Truncated {
		t.Fatal("零值发生变化")
	}
	if OutcomeCompleted != "completed" {
		t.Fatal("常量漂移")
	}
}
