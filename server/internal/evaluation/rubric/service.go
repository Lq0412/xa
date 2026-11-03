package rubric

import "context"

// Service 提供评价规则管理用例。
type Service interface {
	Get(ctx context.Context, rubricID string) (Rubric, error)
	Save(ctx context.Context, rubric Rubric) error
}
