package builtin

import (
	"testing"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/tool"
)

// TestKnowledgeRequirement 验证模型工具目录遵守知识绑定与显式关闭，不能只靠提示词限制。
func TestKnowledgeRequirement(t *testing.T) {
	r := tool.NewRegistry()
	if err := r.Register(NewKbSearch(nil)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		knowledge []binding.KnowledgeBinding
		tools     []binding.ToolBinding
		want      bool
	}{
		{name: "未绑定"},
		{name: "禁用绑定", knowledge: []binding.KnowledgeBinding{{KnowledgeBaseID: "kb_one"}}},
		{name: "绑定后可用", knowledge: []binding.KnowledgeBinding{{KnowledgeBaseID: "kb_one", Enabled: true}}, want: true},
		{name: "显式关闭", knowledge: []binding.KnowledgeBinding{{KnowledgeBaseID: "kb_one", Enabled: true}}, tools: []binding.ToolBinding{{Name: "kb_search", Enabled: false}}},
		{name: "无绑定强行开启", tools: []binding.ToolBinding{{Name: "kb_search", Enabled: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := r.Resolve(tool.ResolveInput{Knowledge: tc.knowledge, Bindings: tc.tools, CanApprove: true})
			if err != nil {
				t.Fatal(err)
			}
			entry, _ := set.EntryByName("kb_search")
			if entry.Enabled != tc.want || (len(set.ModelTools()) > 0) != tc.want {
				t.Fatalf("模型工具目录不符合绑定: %+v", entry)
			}
		})
	}
}
