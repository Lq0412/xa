//go:build integration

package bootstrap_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	"linq-bs/server/internal/agent/runtime"
)

// TestMemoryRecallRegression 复现 memory_v2 场景 09：无关问题不注入召回段，相关查询能召回已植入事实。
func TestMemoryRecallRegression(t *testing.T) {
	db, agents, _, mem, owner, _ := testToolsEnv(t)
	primaryList, err := agents.List(t.Context(), owner)
	if err != nil || len(primaryList) == 0 {
		t.Fatal(err)
	}
	primary := primaryList[0]
	fact := "用户模块化验收锚点：" + uuid.NewString()
	raw, _ := json.Marshal(integrationVector(fact))
	memID := "mem_" + uuid.NewString()
	if _, err := db.Exec(t.Context(), `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,agent_id,namespace,subject_key,content,kind,importance,status,source,embedding,sensitive)
VALUES ($6,$1,$2,'agent',$3,'profile','recall.anchor',$4,'fact',9,'active','tool',$5::vector,false)`,
		owner.WorkspaceID, owner.UserID, primary.ID, fact, string(raw), memID); err != nil {
		t.Fatal(err)
	}
	irrelevant := definition.MemoryPolicy{Enabled: true, CoreBudgetTokens: 1, RecallTopK: 5, MinRelevance: 0.9, DecayEnabled: true}
	details := []runtime.Segment{}
	module := memory.Module{Service: mem, Actor: owner, AgentID: primary.ID, Current: "解释量子色动力学的渐近自由", Policy: irrelevant, Details: &details}
	parts, err := module.Context(t.Context())
	if err != nil || len(parts) != 0 {
		t.Fatalf("无关问题不应注入记忆: parts=%v err=%v", parts, err)
	}
	relevant := definition.MemoryPolicy{Enabled: true, CoreBudgetTokens: 120, RecallTopK: 5, MinRelevance: 0.35, DecayEnabled: true}
	module.Policy = relevant
	module.Current = fact
	parts, err = module.Context(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range parts {
		if strings.Contains(p, fact) {
			found = true
		}
	}
	if !found {
		t.Fatalf("相关查询应召回事实: %v", parts)
	}
}
