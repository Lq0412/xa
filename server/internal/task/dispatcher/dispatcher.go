package dispatcher

import "context"

// Dispatcher 负责领取、重试和确认异步任务。
type Dispatcher interface {
	Dispatch(ctx context.Context) error
}
