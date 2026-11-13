package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
)

// ValidateBindings 在保存 Agent 版本时检查知识库；绑定不会改变知识库的所有权和可见性。
func (s *Service) ValidateBindings(ctx context.Context, actor auth.Principal, visibility definition.Visibility, bindings []binding.KnowledgeBinding) error {
	seen := make(map[string]bool, len(bindings))
	for _, b := range bindings {
		if strings.TrimSpace(b.KnowledgeBaseID) == "" || seen[b.KnowledgeBaseID] {
			return fmt.Errorf("知识库绑定 ID 不能为空或重复: %w", definition.ErrInvalid)
		}
		seen[b.KnowledgeBaseID] = true
		// Filter 保留原有字段形态；本期没有过滤表达式能力，拒绝静默忽略约束。
		if len(b.Filter) > 0 {
			return fmt.Errorf("知识库绑定暂不支持 filter，请留空: %w", definition.ErrInvalid)
		}
		base, err := s.Get(ctx, actor, b.KnowledgeBaseID)
		if err != nil {
			var f *Fault
			if errors.As(err, &f) && f.Code == "not_found" {
				return fmt.Errorf("知识库不存在或不可访问: %w", definition.ErrInvalid)
			}
			return fmt.Errorf("校验知识库绑定: %w", err)
		}
		if base.Visibility != string(visibility) {
			return fmt.Errorf("私有知识库只能绑定自己的私有 Agent，公共知识库只能绑定公共 Agent: %w", definition.ErrInvalid)
		}
	}
	return nil
}
