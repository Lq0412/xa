package fake

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"linq-bs/server/internal/tool"
)

// Register 向注册表装配仅用于测试的假工具；生产启动路径不得调用。
func Register(r *tool.Registry) error {
	tools := []tool.Tool{
		&Echo{}, &Delay{}, &Fail{}, &Huge{}, &ReadCounter{}, &WriteCounter{}, &NeedsApproval{},
	}
	for _, t := range tools {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

var echoSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"message":{"type":"string"}}}`)
var delaySchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"sleep_ms":{"type":"integer","minimum":0,"maximum":60000}}}`)
var failSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"code":{"type":"string"}}}`)
var hugeSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"size":{"type":"integer","minimum":1,"maximum":200000}}}`)

// Echo 返回固定或可配置的字符串。
type Echo struct{}

func (Echo) Spec() tool.Spec {
	return tool.Spec{Name: "fake_echo", Version: "1", Title: "测试回显", InputSchema: echoSchema,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: false, DefaultTimeout: time.Second, MaxCallsPerRun: 64, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (Echo) Execute(_ context.Context, call tool.Call, _ tool.Env) (tool.Result, error) {
	var in struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(call.Arguments, &in)
	if in.Message == "" {
		in.Message = "ok"
	}
	return tool.Result{Data: map[string]any{"message": in.Message}, Display: tool.Display{Summary: in.Message}}, nil
}

// Delay 支持可控延迟或超时。
type Delay struct{}

func (Delay) Spec() tool.Spec {
	return tool.Spec{Name: "fake_delay", Version: "1", Title: "测试延迟", InputSchema: delaySchema,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: false, DefaultTimeout: time.Minute, MaxCallsPerRun: 16, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (Delay) Execute(ctx context.Context, call tool.Call, _ tool.Env) (tool.Result, error) {
	var in struct {
		SleepMS int `json:"sleep_ms"`
	}
	_ = json.Unmarshal(call.Arguments, &in)
	if in.SleepMS <= 0 {
		return tool.Result{Data: map[string]any{"slept_ms": 0}}, nil
	}
	timer := time.NewTimer(time.Duration(in.SleepMS) * time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		return tool.Result{}, &tool.Error{Code: "tool_timeout", Message: "等待被取消", Retryable: true}
	case <-timer.C:
		return tool.Result{Data: map[string]any{"slept_ms": in.SleepMS}}, nil
	}
}

// Fail 返回业务错误或系统错误。
type Fail struct{}

func (Fail) Spec() tool.Spec {
	return tool.Spec{Name: "fake_error", Version: "1", Title: "测试错误", InputSchema: failSchema,
		Effect: tool.EffectRead, DefaultEnabled: false, DefaultTimeout: time.Second, MaxCallsPerRun: 16, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (Fail) Execute(_ context.Context, call tool.Call, _ tool.Env) (tool.Result, error) {
	var in struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(call.Arguments, &in)
	if in.Code == "system" {
		return tool.Result{}, errors.New("模拟系统错误")
	}
	return tool.Result{}, &tool.Error{Code: "failed", Message: "模拟失败", Retryable: true}
}

// Huge 生成超长 JSON 字符串。
type Huge struct{}

func (Huge) Spec() tool.Spec {
	return tool.Spec{Name: "fake_huge", Version: "1", Title: "测试长结果", InputSchema: hugeSchema,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: false, DefaultTimeout: 5 * time.Second, MaxCallsPerRun: 4, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (Huge) Execute(_ context.Context, call tool.Call, _ tool.Env) (tool.Result, error) {
	var in struct {
		Size int `json:"size"`
	}
	_ = json.Unmarshal(call.Arguments, &in)
	if in.Size <= 0 {
		in.Size = 50000
	}
	return tool.Result{Data: map[string]any{"blob": strings.Repeat("x", in.Size)}}, nil
}

var readCalls atomic.Int64
var writeCalls atomic.Int64

// ReadCounter 只读计数器。
type ReadCounter struct{}

func (ReadCounter) Spec() tool.Spec {
	return tool.Spec{Name: "fake_read", Version: "1", Title: "测试只读", InputSchema: echoSchema,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: false, DefaultTimeout: time.Second, MaxCallsPerRun: 64, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (ReadCounter) Execute(context.Context, tool.Call, tool.Env) (tool.Result, error) {
	n := readCalls.Add(1)
	return tool.Result{Data: map[string]any{"count": n}}, nil
}

// WriteCounter 写效果计数器。
type WriteCounter struct{}

func (WriteCounter) Spec() tool.Spec {
	return tool.Spec{Name: "fake_write", Version: "1", Title: "测试写入", InputSchema: echoSchema,
		Effect: tool.EffectWrite, DefaultEnabled: false, DefaultTimeout: time.Second, MaxCallsPerRun: 64, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true}}
}

func (WriteCounter) Execute(context.Context, tool.Call, tool.Env) (tool.Result, error) {
	n := writeCalls.Add(1)
	return tool.Result{Data: map[string]any{"count": n}, Effects: []tool.Effect2{{Kind: "fake.write"}}}, nil
}

// NeedsApproval 默认需要审批。
type NeedsApproval struct{}

func (NeedsApproval) Spec() tool.Spec {
	return tool.Spec{Name: "fake_approval", Version: "1", Title: "测试审批", InputSchema: echoSchema,
		Effect: tool.EffectWrite, DefaultEnabled: false, DefaultTimeout: time.Second, MaxCallsPerRun: 8, Available: true,
		Approval: tool.ApprovalPolicy{Default: tool.ApprovalAlways, Overridable: true}}
}

func (NeedsApproval) Execute(context.Context, tool.Call, tool.Env) (tool.Result, error) {
	return tool.Result{Data: map[string]any{"approved": true}}, nil
}

// ResetCounters 清空测试计数器。
func ResetCounters() {
	readCalls.Store(0)
	writeCalls.Store(0)
}

// ReadCount 返回只读调用次数。
func ReadCount() int64 { return readCalls.Load() }

// WriteCount 返回写入调用次数。
func WriteCount() int64 { return writeCalls.Load() }
