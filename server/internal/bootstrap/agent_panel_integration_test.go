//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
)

// 面板验收使用真实 PostgreSQL，覆盖同工作区内的不同所有者。
func panelRequests(t *testing.T) func(string, string) *httptest.ResponseRecorder {
	t.Helper()
	dsn := os.Getenv("XA_PANEL_TEST_DATABASE_URL")
	var db *pgxpool.Pool
	if dsn == "" {
		db = newP5Database(t)
	} else {
		db = panelDatabase(t, dsn)
	}
	_, err := db.Exec(t.Context(), `
INSERT INTO users(id,workspace_id,email,password_hash,display_name)
VALUES ('panel_owner','default','panel_owner@example.com','unused','Owner'),
       ('panel_other','default','panel_other@example.com','unused','Other');
INSERT INTO agent_definitions(id,workspace_id,owner_id,owner_type,name,visibility,status,created_by)
VALUES ('panel_a','default','panel_owner','user','我的助手','private','active','panel_owner'),
       ('panel_b','default','panel_other','user','其他助手','public','active','panel_other');
INSERT INTO agent_versions(id,workspace_id,agent_id,version_number,configuration,config_checksum,created_by)
VALUES ('panel_av','default','panel_a',1,'{}','a','panel_owner'),('panel_bv','default','panel_b',1,'{}','b','panel_other');
INSERT INTO conversations(id,workspace_id,user_id,agent_id)
VALUES ('panel_ac','default','panel_owner','panel_a'),('panel_bc','default','panel_other','panel_b');
INSERT INTO messages(id,conversation_id,role,content,status)
VALUES ('panel_am','panel_ac','user','你好','completed'),('panel_bm','panel_bc','user','你好','completed');
INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,user_message_id,provider,model,limits,status)
VALUES ('panel_ar','default','panel_owner','panel_ac','panel_a','panel_av','panel_ar','panel_am','bailian','qwen3.8-flash','{}','completed'),
       ('panel_br','default','panel_other','panel_bc','panel_b','panel_bv','panel_br','panel_bm','bailian','qwen3.8-flash','{}','completed');
INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,agent_id,namespace,subject_key,content,importance,source,embedding,status)
VALUES ('panel_forgot','default','panel_owner','agent','panel_a','preferences','coffee','喜欢手冲咖啡',6,'auto',array_fill(1::real,ARRAY[1024])::vector,'deleted'),
       ('panel_manual','default','panel_owner','agent','panel_a','preferences','tea','喜欢乌龙茶',6,'auto',array_fill(1::real,ARRAY[1024])::vector,'deleted');
INSERT INTO memory_events(workspace_id,owner_user_id,memory_id,source_ref,op,actor_type,actor_id,before,after,created_at)
VALUES ('default','panel_owner','panel_forgot','panel_am','ADD','agent','','null','{"content":"喜欢手冲咖啡"}','2026-01-01 10:00Z'),
       ('default','panel_owner','panel_forgot','panel_am','DELETE','agent','','{"content":"喜欢手冲咖啡"}','null','2026-01-01 11:00Z'),
       ('default','panel_owner','panel_manual','panel_am','DELETE','user','panel_owner','{"content":"喜欢乌龙茶"}','null','2026-01-01 12:00Z');
INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key,arguments,display,created_at,ended_at)
VALUES ('panel_tool','panel_ar','panel_ac','panel_owner',1,0,'forget','memory_forget','destructive','succeeded','forget','{"memory_id":"panel_forgot"}','{"items":[{"label":"内容","value":"喜欢手冲咖啡"}]}','2026-01-01 11:00Z','2026-01-01 11:00Z'),
       ('panel_time','panel_ar','panel_ac','panel_owner',2,0,'time','current_time','read','succeeded','time','{}','{"summary":"2026-01-01 13:00"}','2026-01-01 13:00Z','2026-01-01 13:00Z');
INSERT INTO run_approvals(id,run_id,owner_user_id,call_id,tool_name,args_hash,display,status,decided_by,expires_at,created_at,decided_at)
VALUES ('panel_approved','panel_ar','panel_owner','forget','memory_forget','unused','{"title":"遗忘记忆","summary":"将删除这条记忆","items":[{"label":"原因","value":"原始预览"}]}','approved','panel_owner','2026-01-01 11:10Z','2026-01-01 11:00Z','2026-01-01 11:01Z'),
       ('panel_rejected','panel_ar','panel_owner','reject','memory_write','unused','{"title":"写入记忆","summary":"写入这条记忆"}','rejected','panel_owner','2026-01-01 14:10Z','2026-01-01 14:00Z','2026-01-01 14:01Z'),
       ('panel_other_pending','panel_br','panel_other','other','memory_forget','unused','{"title":"其他人的内容"}','pending',NULL,'2026-01-01 14:10Z','2026-01-01 14:00Z',NULL);
`)
	if err != nil {
		t.Fatal(err)
	}
	handler := &agenthttp.RunHandler{Runs: &run.RunService{DB: db}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/{agent_id}/activity", handler.AgentActivity)
	mux.HandleFunc("GET /api/v1/approvals", handler.UserApprovals)
	mux.HandleFunc("GET /api/v1/admin/approvals", handler.ApprovalRecords)
	return func(user, path string) *httptest.ResponseRecorder {
		principal := auth.Principal{UserID: user, WorkspaceID: "default", Roles: []string{"user"}}
		req := httptest.NewRequestWithContext(auth.WithPrincipal(t.Context(), principal), http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}
}

// panelDatabase 允许单独验收面板接口，仅需要 PostgreSQL。
func panelDatabase(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	cfg = cfg.Copy()
	cfg.ConnConfig.Database = "panel_" + strings.ToLower(rand.Text())
	id := pgx.Identifier{cfg.ConnConfig.Database}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+id); err != nil {
			t.Error(err)
		}
	})
	db, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err := db.Exec(t.Context(), "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPanelUserApprovals(t *testing.T) {
	get := panelRequests(t)
	const path = "/api/v1/approvals"
	w := get("panel_owner", path+"?agent_id=panel_a")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "将删除这条记忆") || !strings.Contains(body, `"status":"approved"`) ||
		!strings.Contains(body, `"decided_at":"2026-01-01`) || strings.Contains(body, "example.com") ||
		strings.Contains(body, "原始预览") || strings.Contains(body, "其他人的内容") {
		t.Fatalf("确认摘要: %d %s", w.Code, body)
	}
	for _, query := range []string{"?agent_id=panel_b", "?agent_id=missing"} {
		w := get("panel_owner", path+query)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"approvals":[]`) {
			t.Fatalf("非本人确认记录: %d %s", w.Code, w.Body)
		}
	}
	if w := get("panel_owner", "/api/v1/admin/approvals"); w.Code != 403 {
		t.Fatalf("非管理员查询: %d", w.Code)
	}
}

func TestPanelAgentActivity(t *testing.T) {
	get := panelRequests(t)
	const path = "/api/v1/agents/panel_a/activity"
	type page struct {
		Items  []struct{ Title, Result string }
		Cursor string `json:"next_cursor"`
	}
	read := func(path string) page {
		t.Helper()
		w := get("panel_owner", path)
		var result page
		if w.Code != 200 {
			t.Fatalf("动态: %d %s", w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := read(path + "?limit=2")
	if len(first.Items) != 2 || first.Cursor == "" || first.Items[0].Result != "你已拒绝" || first.Items[1].Title != "查了一次时间" {
		t.Fatalf("第一页: %+v", first)
	}
	second := read(path + "?limit=3&cursor=" + url.QueryEscape(first.Cursor))
	if len(second.Items) != 3 || second.Cursor != "" || second.Items[0].Result != "自己删除 · 喜欢乌龙茶" ||
		second.Items[1].Result != "你已确认 · 喜欢手冲咖啡" || second.Items[2].Title != "记住了" {
		t.Fatalf("第二页: %+v", second)
	}
	for _, path := range []string{"/api/v1/agents/panel_b/activity", "/api/v1/agents/missing/activity"} {
		if w := get("panel_owner", path); w.Code != 404 {
			t.Fatalf("非本人动态: %d", w.Code)
		}
	}
	if w := get("panel_other", path); w.Code != 404 {
		t.Fatalf("他人请求: %d", w.Code)
	}
}
