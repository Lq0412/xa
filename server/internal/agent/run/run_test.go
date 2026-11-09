package run

import (
	"context"
	"testing"
)

// 编译期固定方法表达式，避免测试实现掩盖接口漂移。
var _ func(Engine, context.Context, *RunContext) (Outcome, error) = Engine.Execute

func TestEngineContract(t *testing.T) {
	var outcome Outcome
	if outcome.Kind != "" || outcome.Truncated {
		t.Fatal("骨架零值发生变化")
	}
}
