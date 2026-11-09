package run

import (
	"encoding/json"
	"fmt"

	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/tool"
)

// 拒绝码常量，顺序与 §6.4 一致。
const (
	RejectTooManyCallsInStep  = "too_many_calls_in_step"
	RejectUnknownTool         = "unknown_tool"
	RejectToolUnavailable     = "tool_unavailable"
	RejectNotAllowedThisStep  = "not_allowed_this_step"
	RejectToolBudgetExhausted = "tool_budget_exhausted"
	RejectToolCallLimit       = "tool_call_limit"
	RejectInvalidArguments    = "invalid_arguments"
	RejectSchemaViolation     = "schema_violation"
	RejectRepeatedCall        = "repeated_call"
)

// PlannedCall 描述一步内单个工具调用的校验结果。
type PlannedCall struct {
	Call         provider.ToolCall
	Accepted     bool
	ErrorCode    string
	ErrorMessage string
	CountsBudget bool
}

// CallPlan 是一步内全部调用的校验计划。
type CallPlan struct {
	Items []PlannedCall
}

// AcceptedCalls 返回被接受的调用列表。
func (p CallPlan) AcceptedCalls() []PlannedCall {
	out := make([]PlannedCall, 0)
	for _, it := range p.Items {
		if it.Accepted {
			out = append(out, it)
		}
	}
	return out
}

// Validate 按 §6.4 顺序校验工具调用。
func Validate(calls []provider.ToolCall, rc *RunContext) CallPlan {
	plan := CallPlan{Items: make([]PlannedCall, 0, len(calls))}
	if rc.CallHashes == nil {
		rc.CallHashes = map[string]int{}
	}
	if rc.PerToolCalls == nil {
		rc.PerToolCalls = map[string]int{}
	}
	maxPerStep := rc.Limits.MaxCallsPerStep
	if maxPerStep <= 0 {
		maxPerStep = 8
	}
	for i, call := range calls {
		pc := PlannedCall{Call: call}
		if i >= maxPerStep {
			pc.ErrorCode = RejectTooManyCallsInStep
			pc.ErrorMessage = "本步工具调用过多"
			plan.Items = append(plan.Items, pc)
			continue
		}
		entry, ok := rc.Toolset.EntryByName(call.Name)
		if !ok {
			pc.ErrorCode = RejectUnknownTool
			pc.ErrorMessage = fmt.Sprintf("未知工具 %s", call.Name)
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		if !entry.Enabled || !entry.Spec.Available {
			pc.ErrorCode = RejectToolUnavailable
			if entry.Reason != "" {
				pc.ErrorMessage = entry.Reason
			} else {
				pc.ErrorMessage = "工具不可用"
			}
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		if rc.Limits.MaxToolCalls >= 0 && rc.Budget.ToolCalls >= rc.Limits.MaxToolCalls {
			pc.ErrorCode = RejectToolBudgetExhausted
			pc.ErrorMessage = "本轮工具调用次数已用尽"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		if entry.MaxCalls > 0 && rc.PerToolCalls[call.Name] >= entry.MaxCalls {
			pc.ErrorCode = RejectToolCallLimit
			pc.ErrorMessage = fmt.Sprintf("工具 %s 调用次数已达上限", call.Name)
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		args := []byte(call.Arguments)
		if len(args) == 0 {
			args = []byte("{}")
		}
		if !json.Valid(args) {
			pc.ErrorCode = RejectInvalidArguments
			pc.ErrorMessage = "参数不是合法 JSON 对象"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		var obj any
		if err := json.Unmarshal(args, &obj); err != nil {
			pc.ErrorCode = RejectInvalidArguments
			pc.ErrorMessage = "参数不是合法 JSON 对象"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		if _, isObj := obj.(map[string]any); !isObj {
			pc.ErrorCode = RejectInvalidArguments
			pc.ErrorMessage = "参数必须是 JSON 对象"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		if len(call.Arguments) > tool.MaxArgumentBytes {
			pc.ErrorCode = RejectInvalidArguments
			pc.ErrorMessage = "参数过长"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		violations := tool.ValidateJSON(entry.Spec.InputSchema, args)
		if len(violations) > 0 {
			pc.ErrorCode = RejectSchemaViolation
			pc.ErrorMessage = violations[0].Error()
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		hash := argsHash(call.Name, args)
		rc.CallHashes[hash]++
		if rc.CallHashes[hash] >= 3 {
			pc.ErrorCode = RejectRepeatedCall
			pc.ErrorMessage = "重复调用相同参数"
			pc.CountsBudget = true
			plan.Items = append(plan.Items, pc)
			continue
		}
		pc.Accepted = true
		plan.Items = append(plan.Items, pc)
	}
	return plan
}
