package run

import "errors"

// ErrSubscribeDone 表示订阅已收到终止事件。
var ErrSubscribeDone = errors.New("subscribe 收到终止事件")
