package runtime

import (
	"context"
	"fmt"
	"strings"

	"linq-bs/server/internal/tool"
)

// Tools 在 system 段注入工具使用守则。
type Tools struct {
	Toolset tool.Toolset
}

func (t Tools) Enabled() bool {
	for _, e := range t.Toolset.Entries {
		if e.Enabled && e.Spec.Available {
			return true
		}
	}
	return false
}

// Context 返回每个可用工具一行的使用时机说明。
func (t Tools) Context(context.Context) ([]string, error) {
	lines := []string{"工具使用守则："}
	for _, e := range t.Toolset.Entries {
		if !e.Enabled || !e.Spec.Available {
			continue
		}
		title := e.Spec.Title
		if title == "" {
			title = e.Spec.Name
		}
		lines = append(lines, fmt.Sprintf("- %s（%s）：%s", title, e.Spec.Name, firstSentence(e.Spec.Description)))
	}
	if len(lines) == 1 {
		return nil, nil
	}
	text := strings.Join(lines, "\n")
	if EstimateTokens(text) > 200 {
		text = TruncateTokens(text, 200)
	}
	return []string{text}, nil
}

func firstSentence(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return "按场景调用"
	}
	if idx := strings.IndexAny(desc, "。.\n"); idx > 0 {
		return desc[:idx]
	}
	if len(desc) > 80 {
		return desc[:80] + "…"
	}
	return desc
}
