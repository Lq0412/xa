package scoring

import (
	"context"

	"linq-bs/server/internal/evaluation"
)

// Input 描述一次评价任务的输入。
type Input struct {
	ConversationID string
	TurnID         string
	RubricID       string
	Content        string
}

// Service 执行语法、发音和综合评分。
type Service interface {
	Score(ctx context.Context, input Input) (evaluation.Result, error)
}
