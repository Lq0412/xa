package run

import "strings"

// Emitter 将 v2 事件投影为 v1 SSE 回调（delta / 待分隔符）。
type Emitter struct {
	emit             func(string) error
	pendingSeparator bool
	hadContent       bool
}

// NewV1Emitter 绑定 v1 delta 回调。
func NewV1Emitter(emit func(string) error) *Emitter {
	return &Emitter{emit: emit}
}

// PendingSeparator 标记下一步非空文本前应插入 \n\n。
func (e *Emitter) PendingSeparator(sep string) {
	if sep == "" {
		sep = "\n\n"
	}
	e.pendingSeparator = true
}

// TextDelta 下发可见文本增量；必要时先补分隔符。
func (e *Emitter) TextDelta(delta string) error {
	if delta == "" {
		return nil
	}
	if e.pendingSeparator && e.hadContent && e.emit != nil {
		if err := e.emit("\n\n"); err != nil {
			return err
		}
		e.pendingSeparator = false
	}
	if e.emit == nil {
		return nil
	}
	e.hadContent = true
	return e.emit(delta)
}

// JoinedVisible 将多段文本按 \n\n 拼接，供断言与持久化对齐。
func JoinedVisible(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}
