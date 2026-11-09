package definition_test

import (
	"errors"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
)

// TestToolValidationInvalid 使用真实注册表，校验错误分类与字段提示。
func TestToolValidationInvalid(t *testing.T) {
	s := definition.NewService(nil)
	s.ToolValidator = tool.NewRegistry().ValidateBindings
	err := s.Validate(t.Context(), auth.Principal{UserID: "user", WorkspaceID: "default"}, definition.SaveInput{Configuration: definition.Configuration{Tools: []binding.ToolBinding{{Name: "missing"}}}})
	if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), "name") {
		t.Fatal("工具错误未分类为 ErrInvalid 或丢失字段原因")
	}
}
