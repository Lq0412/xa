//go:build integration

package bootstrap_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/tool"
	"linq-bs/server/internal/tool/builtin"
)

// TestSensitiveMemoryNotInjectedIntoContext 敏感记忆不得进入对话上下文的 core 或 recall 段。
func TestSensitiveMemoryNotInjectedIntoContext(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	primaryList, err := agents.List(t.Context(), owner)
	if err != nil || len(primaryList) == 0 {
		t.Fatal(err)
	}
	primary := primaryList[0]
	suffix := uuid.NewString()
	sensitiveText := "用户有高血压，正在服药-" + suffix
	controlText := "用户喜欢手冲咖啡-" + suffix

	insert := func(content string, sensitive bool) string {
		raw, _ := json.Marshal(integrationVector(content))
		id := "mem_" + uuid.NewString()
		subject := "sensitive." + suffix
		if !sensitive {
			subject = "control." + suffix
		}
		if _, err := db.Exec(t.Context(), `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,namespace,subject_key,content,kind,importance,status,source,embedding,sensitive)
VALUES ($1,$2,$3,'user','profile',$4,$5,'fact',9,'active','tool',$6::vector,$7)`,
			id, owner.WorkspaceID, owner.UserID, subject, content, string(raw), sensitive); err != nil {
			t.Fatal(err)
		}
		return id
	}
	sensitiveID := insert(sensitiveText, true)
	controlID := insert(controlText, false)

	policy := definition.MemoryPolicy{Enabled: true, CoreBudgetTokens: 200, RecallTopK: 5, MinRelevance: 0.35, DecayEnabled: true}
	assertNoSensitive := func(parts []string, details []runtime.Segment) {
		t.Helper()
		for _, p := range parts {
			if strings.Contains(p, sensitiveText) {
				t.Fatalf("parts 含敏感文案: %q", p)
			}
		}
		for _, d := range details {
			if slices.Contains(d.ItemIDs, sensitiveID) {
				t.Fatalf("段 %s 含敏感记忆 id", d.Name)
			}
		}
	}

	details := []runtime.Segment{}
	module := memory.Module{Service: mem, Actor: owner, AgentID: primary.ID, Current: sensitiveText, Policy: policy, Details: &details}
	parts, err := module.Context(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertNoSensitive(parts, details)

	scored, err := mem.Recall(t.Context(), owner, primary.ID, sensitiveText, memory.RecallOptions{
		TopK: 5, MinRelevance: 0.35, Touch: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range scored {
		if row.ID == sensitiveID {
			t.Fatalf("Recall 不应返回敏感记忆: %+v", scored)
		}
	}

	details = []runtime.Segment{}
	module.Current = controlText
	module.Details = &details
	parts, err = module.Context(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	foundControl := false
	for _, p := range parts {
		if strings.Contains(p, controlText) {
			foundControl = true
		}
	}
	if !foundControl {
		t.Fatalf("对照记忆应被召回: parts=%v controlID=%s", parts, controlID)
	}
}

// TestMemorySearchOmitsSensitive memory_search 不得返回敏感条目。
func TestMemorySearchOmitsSensitive(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	primaryList, err := agents.List(t.Context(), owner)
	if err != nil || len(primaryList) == 0 {
		t.Fatal(err)
	}
	primary := primaryList[0]
	suffix := uuid.NewString()
	sensitiveText := "用户有高血压，正在服药-" + suffix
	controlText := "用户喜欢手冲咖啡-" + suffix

	for _, row := range []struct {
		content   string
		sensitive bool
	}{
		{sensitiveText, true},
		{controlText, false},
	} {
		raw, _ := json.Marshal(integrationVector(row.content))
		id := "mem_" + uuid.NewString()
		subject := "sensitive." + suffix
		if !row.sensitive {
			subject = "control." + suffix
		}
		if _, err := db.Exec(t.Context(), `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,namespace,subject_key,content,kind,importance,status,source,embedding,sensitive)
VALUES ($1,$2,$3,'user','profile',$4,$5,'fact',9,'active','tool',$6::vector,$7)`,
			id, owner.WorkspaceID, owner.UserID, subject, row.content, string(raw), row.sensitive); err != nil {
			t.Fatal(err)
		}
	}

	arg, _ := json.Marshal(map[string]string{"query": controlText})
	search := builtin.NewMemorySearch(mem)
	res, err := search.Execute(t.Context(), tool.Call{
		Name: "memory_search", Arguments: arg,
	}, tool.Env{Principal: owner, AgentID: primary.ID})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	rawItems, _ := data["items"].([]map[string]any)
	items := rawItems
	for _, it := range items {
		content, _ := it["content"].(string)
		if strings.Contains(content, sensitiveText) {
			t.Fatalf("memory_search 返回了敏感条目: %+v", items)
		}
	}
	foundControl := false
	for _, it := range items {
		content, _ := it["content"].(string)
		if strings.Contains(content, controlText) {
			foundControl = true
		}
	}
	if !foundControl {
		t.Fatalf("memory_search 应返回对照条目: %+v", items)
	}
}
