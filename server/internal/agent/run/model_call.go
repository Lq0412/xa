package run

import (
	"context"
	"errors"
	"time"

	"linq-bs/server/internal/infra/provider"
)

// callModelWithRetry 在可重试错误时最多重试 2 次。
func callModelWithRetry(ctx context.Context, rc *RunContext, req provider.ChatRequest, sink func(provider.StreamEvent) error) (provider.Response, error) {
	attempt := 0
	for {
		resp, err := rc.Provider.Chat(ctx, req, sink)
		if err == nil {
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
