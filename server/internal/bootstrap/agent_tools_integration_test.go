//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/memory"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/system/user"
	"linq-bs/server/internal/tool"
)

func testToolsEnv(t *testing.T) (*pgxpool.Pool, definition.Service, *tool.Registry, *memory.Service, auth.Principal, auth.Principal) {
	t.Helper()
	cfg := loadConfig(t)
	db, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	users := user.NewService(user.NewPGRepository(db))
	u1, err := users.Register(t.Context(), user.Credentials{Email: "t25-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	u2, err := users.Register(t.Context(), user.Credentials{Email: "t25b-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	p1 := auth.Principal{UserID: u1.ID, WorkspaceID: u1.WorkspaceID, Roles: []string{"user"}}
	p2 := auth.Principal{UserID: u2.ID, WorkspaceID: u2.WorkspaceID, Roles: []string{"user"}}
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	agents.ToolValidator = nil
	model := provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)
	mem := &memory.Service{DB: db, Agents: agents, Model: model, QueryEmbed: func(_ context.Context, query string) ([]float32, error) {
		return integrationVector(query), nil
	}}
	reg, err := bootstrap.NewToolRegistry(mem, db)
	if err != nil {
		t.Fatal(err)
	}
	agents.ToolValidator = reg.ValidateBindings
	return db, agents, reg, mem, p1, p2
}

func TestAgentToolsAPI(t *testing.T) {
	_, agents, reg, _, owner, other := testToolsEnv(t)
	h := &agenthttp.ToolsHandler{Registry: reg, Agents: agents}
	agentsOnDB, err := agents.List(t.Context(), owner)
	if err != nil || len(agentsOnDB) == 0 {
		t.Fatal("无 Agent")
	}
	primary := agentsOnDB[0]
	v, err := agents.GetVersion(t.Context(), owner, primary.ID, primary.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.Catalog(w, httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil).WithContext(auth.WithPrincipal(t.Context(), owner)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var cat struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil || len(cat.Tools) < 7 {
		t.Fatal("目录过短", w.Body.String())
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+primary.ID+"/tools", nil).WithContext(auth.WithPrincipal(t.Context(), other))
	req.SetPathValue("agent_id", primary.ID)
	h.GetAgentTools(w, req)
	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Fatalf("非 owner 应拒绝: %d", w.Code)
	}
	body, _ := json.Marshal(map[string]any{
		"expected_version_id": v.ID,
		"tools": []binding.ToolBinding{
			{Name: "current_time", Enabled: true, Configuration: json.RawMessage(`{"timezone":"UTC"}`)},
			{Name: "memory_forget", Enabled: true, Approval: "always"},
		},
		"runtime_limits": json.RawMessage(`{"max_tool_calls":8}`),
	})
	w = httptest.NewRecorder()
	put := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+primary.ID+"/tools", strings.NewReader(string(body)))
	put = put.WithContext(auth.WithPrincipal(t.Context(), owner))
	put.SetPathValue("agent_id", primary.ID)
	h.PutAgentTools(w, put)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var saved map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved["current_version_id"] == v.ID {
		t.Fatal("应生成新版本")
	}
	hash1, _ := saved["toolset_hash"].(string)
	w = httptest.NewRecorder()
	put2 := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+primary.ID+"/tools", strings.NewReader(string(body)))
	put2 = put2.WithContext(auth.WithPrincipal(t.Context(), owner))
	put2.SetPathValue("agent_id", primary.ID)
	h.PutAgentTools(w, put2)
	if w.Code != http.StatusConflict {
		t.Fatalf("版本冲突期望 409: %d", w.Code)
	}
	bad, _ := json.Marshal(map[string]any{
		"expected_version_id": saved["current_version_id"],
		"tools":               []binding.ToolBinding{{Name: "not_registered", Enabled: true}},
	})
	w = httptest.NewRecorder()
	put3 := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+primary.ID+"/tools", strings.NewReader(string(bad)))
	put3 = put3.WithContext(auth.WithPrincipal(t.Context(), owner))
	put3.SetPathValue("agent_id", primary.ID)
	h.PutAgentTools(w, put3)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法 name 应 422: %d", w.Code)
	}
	w = httptest.NewRecorder()
	get := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+primary.ID+"/tools", nil).WithContext(auth.WithPrincipal(t.Context(), owner))
	get.SetPathValue("agent_id", primary.ID)
	h.GetAgentTools(w, get)
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if h2, _ := saved["toolset_hash"].(string); h2 != hash1 {
		t.Fatal("同配置 hash 应稳定")
	}
}

func TestMemoryToolScopes(t *testing.T) {
	db, agents, _, mem, owner, other := testToolsEnv(t)
	primaryList, err := agents.List(t.Context(), owner)
	if err != nil || len(primaryList) == 0 {
		t.Fatal(err)
	}
	primary := primaryList[0]
	v, err := agents.GetVersion(t.Context(), owner, primary.ID, primary.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := agents.Save(t.Context(), owner, definition.SaveInput{
		Name: "子Agent" + rand.Text()[:4], Visibility: definition.VisibilityPrivate, Configuration: v.Configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceRef := seedUserMessage(t, t.Context(), db, owner, primary.ID)
	vector := integrationVector("用户喜欢绿茶")
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	item, op, err := mem.ToolWriteTx(t.Context(), tx, owner, memory.ToolWriteInput{
		AgentID: primary.ID, IsPrimary: primary.IsPrimary, SourceRef: sourceRef, Content: "用户喜欢绿茶", Namespace: "preferences", Importance: 7,
	}, vector)
	if err != nil || op == "noop" || item.Scope != "user" || item.Source != "tool" {
		_ = tx.Rollback(t.Context())
		t.Fatalf("主 Agent 写入: %v %+v", err, item)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	childSource := seedUserMessage(t, t.Context(), db, owner, child.Definition.ID)
	vector2 := integrationVector("子 Agent 私有事实")
	tx, err = db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	item2, _, err := mem.ToolWriteTx(t.Context(), tx, owner, memory.ToolWriteInput{
		AgentID: child.Definition.ID, IsPrimary: false, SourceRef: childSource, Content: "子 Agent 私有事实", Namespace: "profile", Importance: 6,
	}, vector2)
	if err != nil || item2.Scope != "agent" || item2.AgentID == nil || *item2.AgentID != child.Definition.ID {
		t.Fatalf("非主 Agent scope: %+v err=%v", item2, err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = seedUserEditedMemory(t, t.Context(), db, owner, "profile", "manual.fact", "用户手填")
	tx, err = db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = mem.ToolWriteTx(t.Context(), tx, owner, memory.ToolWriteInput{
		AgentID: primary.ID, IsPrimary: primary.IsPrimary, SourceRef: sourceRef, Content: "自动改写", Namespace: "profile", SubjectKey: "manual.fact", Importance: 5,
	}, vector)
	var toolErr *tool.Error
	if err == nil || !errors.As(err, &toolErr) || toolErr.Code != "conflict_user_edited" {
		t.Fatalf("应拒绝 user_edited 冲突: %v", err)
	}
	_ = tx.Rollback(t.Context())
	tx, err = db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mem.ToolForgetTx(t.Context(), tx, other, primary.ID, item.ID, sourceRef, "越权删除")
	if err == nil {
		t.Fatal("用户 B 不能删除用户 A 的记忆")
	}
	_ = tx.Rollback(t.Context())
}
