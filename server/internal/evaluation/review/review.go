package review

import "context"

// Service 负责人工复核和评分结果查询。
type Service interface {
	Review(ctx context.Context, resultID string, decision string, note string) error
}
