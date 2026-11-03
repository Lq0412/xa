package rerank

import "context"

// Client 对检索候选进行重排序。
type Client interface {
	Rank(ctx context.Context, query string, documents []string) ([]float64, error)
}
