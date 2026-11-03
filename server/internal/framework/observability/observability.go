package observability

import "context"

// Logger 为各模块提供结构化日志接口。
type Logger interface {
	Info(ctx context.Context, message string, fields map[string]any)
	Error(ctx context.Context, message string, fields map[string]any)
}

// Metrics 为调用、延迟和错误提供指标边界。
type Metrics interface {
	Count(name string, value float64, labels map[string]string)
	Observe(name string, value float64, labels map[string]string)
}
