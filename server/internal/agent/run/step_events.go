package run

import (
	"context"

	"linq-bs/server/internal/infra/provider"
)

func emitStepCompleted(ctx context.Context, rc *RunContext, resp provider.Response) {
	if rc.Emitter == nil {
		return
	}
	reasoning := 0
	_ = rc.Emitter.Emit(ctx, EventStepCompleted, map[string]any{
		"step": rc.Step, "finish_reason": string(resp.FinishReason),
		"usage": map[string]any{
			"prompt": resp.Usage.PromptTokens, "completion": resp.Usage.CompletionTokens,
			"reasoning": reasoning, "cached": resp.Usage.CachedTokens,
		},
		"anomalies": resp.Anomalies,
	})
}
