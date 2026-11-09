//go:build integration

package bootstrap_test

import (
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/system/user"
	"linq-bs/server/internal/tool"
)

// TestStrictToolsMemorySave 验证真实仓储整份 Save 的旧配置与空 tools 兼容，历史版本不可变。
func TestStrictToolsMemorySave(t *testing.T) {
	cfg := loadConfig(t)
	db, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, err := user.NewService(user.NewPGRepository(db)).Register(t.Context(), user.Credentials{Email: "r1-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupModuleUser(t, db, u.ID)
	p := auth.Principal{UserID: u.ID, WorkspaceID: u.WorkspaceID, Roles: []string{"user"}}
	s := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	s.ToolValidator = tool.NewRegistry().ValidateBindings
	agents, err := (&definition.Library{DB: db}).List(t.Context(), p)
	if err != nil || len(agents) == 0 {
		t.Fatal("主 Agent 未创建")
	}
	a := agents[0]
	v, err := s.GetVersion(t.Context(), p, a.ID, a.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow(t.Context(), `SELECT configuration::text FROM agent_versions WHERE id=$1`, v.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, tools := range [][]binding.ToolBinding{nil, {}} {
		input := definition.SaveInput{AgentID: a.ID, ExpectedVersionID: v.ID, Name: a.Name, Visibility: a.Visibility, Configuration: v.Configuration}
		input.Configuration.Tools = tools
		input.Configuration.MemoryPolicy = json.RawMessage(`{"enabled":false}`)
		saved, err := s.Save(t.Context(), p, input)
		if err != nil {
			t.Fatal(err)
		}
		var after string
		if err := db.QueryRow(t.Context(), `SELECT configuration::text FROM agent_versions WHERE id=$1`, v.ID).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Fatal("整份 Save 改写了历史版本")
		}
		v = saved.Version
		if err := db.QueryRow(t.Context(), `SELECT configuration::text FROM agent_versions WHERE id=$1`, v.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		policy, err := definition.ParseMemoryPolicy(v.Configuration.MemoryPolicy)
		if err != nil || policy.Enabled {
			t.Fatal("strict tools 阻断了记忆策略保存")
		}
	}
}
