package run

import (
	"context"
	"errors"
	"time"

	"linq-bs/server/internal/infra/provider"
)

// callModelWithRetry 在可重试错误时最多重试 2 次。
func callModelWithRetry(ctx context.Context, rc *RunContext, req provider.ChatRequest, sink func(provider.StreamEvent) error) (response provider.Response, resultErr error) {
	var span Span
	started := time.Now().UTC()
	if rc.Tracer != nil {
		span = rc.Tracer.Start(rc.Tracer.RootID, "llm", "chat "+req.Model, rc.Step, "", started)
		span.Attributes = map[string]any{"gen_ai.operation.name": "chat", "gen_ai.request.model": req.Model, "gen_ai.provider.name": rc.Run.Provider, "tools_sent": len(req.Tools)}
	}
	attempt := 0
	defer func() {
		if rc.Tracer == nil {
			return
		}
		span.Attributes["attempts"] = attempt + 1
		span.Attributes["retries"] = attempt
		span.Attributes["finish_reason"] = response.FinishReason
		span.Attributes["raw_finish_reason"] = response.RawFinishReason
		span.Attributes["anomalies"] = response.Anomalies
		if response.TTFT > 0 {
			span.Attributes["ttft_ms"] = response.TTFT.Milliseconds()
		}
		span.PromptTokens = &response.Usage.PromptTokens
		span.CompletionTokens = &response.Usage.CompletionTokens
		span.CachedTokens = &response.Usage.CachedTokens
		span.OutputPreview = response.Message.Content
		status, code := "ok", ""
		if resultErr != nil {
			status = "error"
			code = "model_error"
			span.PromptTokens = nil
			span.CompletionTokens = nil
			span.CachedTokens = nil
			if errors.Is(resultErr, context.Canceled) {
				status = "cancelled"
			}
		}
		rc.Tracer.End(span, status, code, time.Now().UTC())
		if err := rc.Tracer.FlushStep(context.WithoutCancel(ctx)); err != nil {
			resultErr = err
		}
	}()
	for {
		resp, err := rc.Provider.Chat(ctx, req, sink)
		if err == nil {
			if rc.Run.TTFTMS == nil && resp.TTFT > 0 {
				ms := resp.TTFT.Milliseconds()
				rc.Run.TTFTMS = &ms
			}
			return resp, nil
		}
		var pe *provider.ProviderError
		if errors.As(err, &pe) {
			if pe.Emitted {
				rc.Emitted = true
			}
			if !retryableProvider(pe) || pe.Emitted || attempt >= 2 {
				return resp, classifyProviderError(pe)
			}
			backoff := providerBackoff(attempt, pe.RetryAfter)
			if rc.RemainingWall() < backoff {
				return resp, classifyProviderError(pe)
			}
			attempt++
			select {
			case <-ctx.Done():
				return resp, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
		return resp, err
	}
}

func retryableProvider(pe *provider.ProviderError) bool {
	switch pe.Kind {
	case provider.ProviderErrorRateLimited, provider.ProviderErrorUnavailable, provider.ProviderErrorTimeout, provider.ProviderErrorStreamBroken:
		return true
	default:
		return false
	}
}

func providerBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 && retryAfter <= 10*time.Second {
		return retryAfter
	}
	if attempt == 0 {
		return time.Second
	}
	return 3 * time.Second
}

func classifyProviderError(pe *provider.ProviderError) error {
	if pe == nil {
		return errors.New("模型调用失败")
	}
	return pe
}
