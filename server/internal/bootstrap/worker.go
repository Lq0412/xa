package bootstrap

import "context"

// Worker 负责组装异步评价、记忆提取和知识摄取任务。
type Worker struct{}

// NewWorker 创建 worker 进程的依赖装配入口。
func NewWorker() *Worker {
	return &Worker{}
}

// Run 启动 worker 进程。
func (w *Worker) Run(ctx context.Context) error {
	panic("TODO: implement worker bootstrap")
}
