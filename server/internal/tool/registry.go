package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"linq-bs/server/internal/agent/binding"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Registry 保存工具实现。P0 只提供注册和查询边界，不装配任何工具。
type Registry struct{ tools map[string]Tool }

func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("工具不能为空")
	}
	spec := t.Spec()
	if !namePattern.MatchString(spec.Name) {
		return fmt.Errorf("工具名称无效: %s", spec.Name)
	}
	if _, exists := r.tools[spec.Name]; exists {
		return fmt.Errorf("工具重复注册: %s", spec.Name)
	}
	r.tools[spec.Name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Catalog() []Spec {
	items := make([]Spec, 0, len(r.tools))
	for _, t := range r.tools {
		items = append(items, t.Spec())
	}
	slices.SortFunc(items, func(a, b Spec) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return items
}

// ValidateBindings 校验配置中的字段和已注册名称；空配置保持兼容。
func (r *Registry) ValidateBindings(bindings []binding.ToolBinding) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if b.Name == "" {
			return fmt.Errorf("工具 name 不能为空")
		}
		if !namePattern.MatchString(b.Name) {
			return fmt.Errorf("工具 name 格式无效")
		}
		t, ok := r.tools[b.Name]
		if !ok {
			return fmt.Errorf("工具 name 未注册")
		}
		if _, ok := seen[b.Name]; ok {
			return fmt.Errorf("工具重复配置: %s", b.Name)
		}
		seen[b.Name] = struct{}{}
		if b.Approval != "" && b.Approval != string(ApprovalNever) && b.Approval != string(ApprovalAlways) {
			return fmt.Errorf("工具 %s 的 approval 无效", b.Name)
		}
		if b.TimeoutMS < 0 || b.TimeoutMS > 60000 {
			return fmt.Errorf("工具 %s 的 timeout_ms 超出范围", b.Name)
		}
		if err := validateBinding(b, t.Spec()); err != nil {
			return fmt.Errorf("工具 %s: %w", b.Name, err)
		}
	}
	return nil
}

// validateBinding 在不可变版本写入前限制权限与配置形状；完整 Schema 值校验留给 P2。
func validateBinding(b binding.ToolBinding, spec Spec) error {
	if b.MaxCalls < 0 || b.MaxCalls > spec.MaxCallsPerRun {
		return fmt.Errorf("max_calls 超出工具上限")
	}
	if !spec.Approval.Overridable && spec.Approval.Default == ApprovalAlways && b.Approval == string(ApprovalNever) {
		return fmt.Errorf("approval 不允许取消强制审批")
	}
	for _, role := range b.AllowedRoles {
		if role != "user" && role != "admin" {
			return fmt.Errorf("allowed_roles 含未知角色")
		}
	}
	if len(b.Configuration) == 0 {
		return nil
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(b.Configuration, &config); err != nil || config == nil {
		return fmt.Errorf("configuration 必须是对象")
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if len(spec.ConfigSchema) > 0 {
		if err := json.Unmarshal(spec.ConfigSchema, &schema); err != nil {
			return fmt.Errorf("configuration 的 ConfigSchema 无效")
		}
	}
	for field := range config {
		if _, ok := schema.Properties[field]; !ok {
			return fmt.Errorf("configuration 含未声明字段")
		}
	}
	return nil
}
