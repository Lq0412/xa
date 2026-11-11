//go:build integration

package bootstrap_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
)

// seedP6Queries 在独立真实数据库建立跨 owner/workspace 的终态与运行中记录。
func seedP6Queries(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(t.Context(), `
INSERT INTO workspaces(id,name) VALUES('p6_other','P6 other');
INSERT INTO users(id,workspace_id,email,password_hash,display_name) VALUES('p6_u1','default','p6_u1@example.com','unused-test-password-hash','P6 owner'),('p6_u2','default','p6_u2@example.com','unused-test-password-hash','P6 other'),('p6_u3','p6_other','p6_u3@example.com','unused-test-password-hash','P6 cross');
INSERT INTO agent_definitions(id,workspace_id,owner_id,owner_type,name,visibility,status,created_by) VALUES('p6_a','default','p6_u1','user','P6 query','private','active','p6_u1'),('p6_a2','p6_other','p6_u3','user','P6 cross','private','active','p6_u3');
INSERT INTO agent_versions(id,workspace_id,agent_id,version_number,configuration,config_checksum,created_by) VALUES('p6_v','default','p6_a',1,'{}','p6','p6_u1'),('p6_v2','p6_other','p6_a2',1,'{}','p6','p6_u3');
INSERT INTO agent_definitions(id,workspace_id,owner_id,owner_type,name,visibility,status,created_by) VALUES('p6_a3','default','p6_u1','user','P6 live','private','active','p6_u1');
INSERT INTO agent_versions(id,workspace_id,agent_id,version_number,configuration,config_checksum,created_by) VALUES('p6_v3','default','p6_a3',1,'{}','p6','p6_u1');
INSERT INTO conversations(id,workspace_id,user_id,agent_id) VALUES('p6_c','default','p6_u1','p6_a'),('p6_c2','p6_other','p6_u3','p6_a2'),('p6_c3','default','p6_u1','p6_a3');
INSERT INTO messages(id,conversation_id,role,content,status) VALUES('p6_m','p6_c','user','查询与清理','completed'),('p6_m2','p6_c2','user','跨 workspace','completed'),('p6_m3','p6_c3','user','运行中','completed'),('p6_answer','p6_c','assistant','结果正文','completed'),('p6_live_answer','p6_c3','assistant','直播正文','pending');
UPDATE messages SET created_at=now()+interval '1 second' WHERE role='assistant';
INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,user_message_id,final_message_id,provider,model,limits,toolset,status,created_at,started_at,ended_at) VALUES
('p6_r1','default','p6_u1','p6_c','p6_a','p6_v','p6_r1','p6_m','p6_answer','bailian','qwen-turbo','{}','{"entries":[{"spec":{"Name":"memory_search"}}]}','completed','2025-01-01','2025-01-01','2025-01-01'),
('p6_r2','default','p6_u1','p6_c','p6_a','p6_v','p6_r2','p6_m',NULL,'bailian','qwen-turbo','{}','[]','completed','2025-01-01','2025-01-01',now()),
('p6_cross','p6_other','p6_u3','p6_c2','p6_a2','p6_v2','p6_cross','p6_m2',NULL,'bailian','qwen-turbo','{}','[]','completed','2025-01-01','2025-01-01',now()),
('p6_live','default','p6_u1','p6_c3','p6_a3','p6_v3','p6_live','p6_m3',NULL,'bailian','qwen-turbo','{}','[]','waiting_approval','2025-01-01','2025-01-01','2025-01-01');
UPDATE messages SET run_id='p6_r1' WHERE id='p6_answer';
UPDATE messages SET run_id='p6_live' WHERE id='p6_live_answer';
INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key,data,display,model_content) VALUES('p6_tool','p6_r1','p6_c','p6_u1',1,0,'p6_call','memory_search','read','succeeded','p6_r1:p6_call','{"retained":true}','{"summary":"工具摘要"}','工具结果');
INSERT INTO run_contexts(run_id,messages,segments,created_at) VALUES('p6_r1','[]','[]','2025-01-01'),('p6_live','[]','[]','2025-01-01');
INSERT INTO run_spans(id,run_id,kind,name,status,attributes,input_preview,output_preview,started_at,ended_at) VALUES('p6_span','p6_r1','run','invoke_agent P6','ok','{"hint":"owner@example.com"}','Bearer demo-sensitive-token','结果','2025-01-01','2025-01-01');
INSERT INTO run_events(run_id,seq,type,payload,created_at) VALUES
('p6_r1',1,'reasoning.delta','{"type":"reasoning.delta","item_id":"reason","delta":"不能进入历史的思考正文"}','2025-01-01'),
('p6_r1',2,'reasoning.completed','{"type":"reasoning.completed","item_id":"reason","duration_ms":12}','2025-01-01'),
('p6_r1',3,'tool.completed','{"type":"tool.completed","call_id":"p6_call","name":"memory_search","status":"succeeded","summary":"工具摘要"}','2025-01-01'),
('p6_r1',4,'message.delta','{"type":"message.delta","delta":"结果正文"}','2025-01-01'),
('p6_r1',5,'run.completed','{"type":"run.completed"}','2025-01-01'),
('p6_live',1,'message.delta','{"type":"message.delta","delta":"直播正文"}','2025-01-01');
`)
	if err != nil {
		t.Fatal(err)
	}
}

// TestP6TraceQueriesAndRetention 不调用模型，验证真实 SQL 权限、分页与删除前快照。
func TestP6TraceQueriesAndRetention(t *testing.T) {
	db := newP5Database(t)
	seedP6Queries(t, db)
	service := &run.RunService{DB: db, Store: run.NewPGStore(db), SnapshotParts: conversation.SnapshotRunParts, Config: config.RunConfig{EventRetention: 168 * time.Hour, ContextRetention: 168 * time.Hour, TraceRetention: 720 * time.Hour}}
	h := &agenthttp.RunHandler{Runs: service}
	owner := auth.Principal{UserID: "p6_u1", WorkspaceID: "default", Roles: []string{"user"}}
	other := auth.Principal{UserID: "p6_u2", WorkspaceID: "default", Roles: []string{"user"}}
	admin := auth.Principal{UserID: "p6_u2", WorkspaceID: "default", Roles: []string{"admin"}}
	request := func(actor auth.Principal, path, id string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(auth.WithPrincipal(t.Context(), actor), http.MethodGet, path, nil)
		req.SetPathValue("run_id", id)
		req.SetPathValue("call_id", "p6_call")
		w := httptest.NewRecorder()
		handler(w, req)
		return w
	}
	for _, endpoint := range []struct {
		suffix  string
		handler http.HandlerFunc
	}{{"", h.GetRun}, {"/spans", h.Spans}, {"/tool-results/p6_call", h.ToolResult}} {
		for _, id := range []string{"p6_r1", "p6_missing"} {
			w := request(other, "/api/v1/runs/"+id+endpoint.suffix, id, endpoint.handler)
			if w.Code != 404 {
				t.Fatalf("非 owner %s: %d %s", endpoint.suffix, w.Code, w.Body)
			}
		}
	}
	if w := request(owner, "/api/v1/admin/runs", "", h.ListRuns); w.Code != 403 {
		t.Fatalf("非 admin: %d", w.Code)
	}
	for _, id := range []string{"p6_cross", "p6_missing"} {
		for _, endpoint := range []struct {
			suffix  string
			handler http.HandlerFunc
		}{{"", h.GetRun}, {"/spans", h.Spans}} {
			if w := request(admin, "/api/v1/admin/runs/"+id+endpoint.suffix, id, endpoint.handler); w.Code != 404 {
				t.Fatalf("跨 workspace: %d %s", w.Code, w.Body)
			}
		}
	}
	for _, query := range []string{"?status=invalid", "?limit=-1", "?limit=101", "?cursor=broken"} {
		if w := request(owner, "/api/v1/runs"+query, "", h.ListRuns); w.Code != 400 {
			t.Fatalf("非法筛选 %s: %d %s", query, w.Code, w.Body)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		w := request(owner, "/api/v1/runs?limit=1&cursor="+url.QueryEscape(cursor), "", h.ListRuns)
		var page run.RunPage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 {
			t.Fatalf("分页: %d %s %v", w.Code, w.Body, err)
		}
		for _, item := range page.Runs {
			id := item["id"].(string)
			if seen[id] {
				t.Fatal("重复分页", id)
			}
			seen[id] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 || seen["p6_cross"] {
		t.Fatal("分页范围错误", seen)
	}
	if w := request(owner, "/api/v1/runs/p6_r1", "p6_r1", h.GetRun); w.Code != 200 || !strings.Contains(w.Body.String(), `"memory_search"`) || !strings.Contains(w.Body.String(), "+08:00") {
		t.Fatalf("工具名称投影: %d %s", w.Code, w.Body)
	}
	if w := request(admin, "/api/v1/admin/runs/p6_r1/spans", "p6_r1", h.Spans); w.Code != 200 || strings.Contains(w.Body.String(), "owner@example.com") || strings.Contains(w.Body.String(), "demo-sensitive-token") || strings.Contains(w.Body.String(), `"data"`) {
		t.Fatalf("管理树脱敏: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "/api/v1/runs/p6_r2/spans", "p6_r2", h.Spans); w.Code != 200 || !strings.Contains(w.Body.String(), `"span":null`) {
		t.Fatalf("旧树: %d %s", w.Code, w.Body)
	}
	chats := &conversation.Service{DB: db}
	before, err := chats.Messages(t.Context(), owner, "p6_c", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Messages) == 0 || len(before.Messages[len(before.Messages)-1].Parts) == 0 {
		t.Fatal("快照前事件未重放")
	}
	// 即使回调已写入快照，随后失败也必须回滚，事件和消息都保持原状。
	service.SnapshotParts = func(ctx context.Context, tx pgx.Tx, id string) error {
		if err := conversation.SnapshotRunParts(ctx, tx, id); err != nil {
			return err
		}
		return errors.New("快照写入后的测试失败")
	}
	if err := service.CleanupRetention(t.Context()); err == nil {
		t.Fatal("快照失败被忽略")
	}
	var atomic bool
	if err := db.QueryRow(t.Context(), `SELECT (SELECT parts IS NULL FROM messages WHERE id='p6_answer') AND (SELECT count(*)=5 FROM run_events WHERE run_id='p6_r1')`).Scan(&atomic); err != nil || !atomic {
		t.Fatal("失败清理未回滚", err)
	}
	service.SnapshotParts = conversation.SnapshotRunParts
	if err := service.CleanupRetention(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := service.CleanupRetention(t.Context()); err != nil {
		t.Fatal("清理不可重入", err)
	}
	var clean, preserved bool
	err = db.QueryRow(t.Context(), `SELECT
(SELECT count(*)=0 FROM run_events WHERE run_id='p6_r1') AND (SELECT count(*)=0 FROM run_contexts WHERE run_id='p6_r1') AND (SELECT attributes='{}'::jsonb AND input_preview='' AND output_preview='' FROM run_spans WHERE id='p6_span') AND (SELECT data IS NULL AND display IS NOT NULL AND model_content='工具结果' FROM tool_results WHERE id='p6_tool'),
(SELECT count(*)=1 FROM run_events WHERE run_id='p6_live') AND (SELECT count(*)=1 FROM run_contexts WHERE run_id='p6_live') AND (SELECT count(*)=1 FROM runs WHERE id='p6_r1')`).Scan(&clean, &preserved)
	if err != nil || !clean || !preserved {
		t.Fatal("清理范围错误", clean, preserved, err)
	}
	after, err := chats.Messages(t.Context(), owner, "p6_c", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	beforeRaw, err := json.Marshal(before.Messages)
	if err != nil {
		t.Fatal(err)
	}
	afterRaw, err := json.Marshal(after.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeRaw) != string(afterRaw) || strings.Contains(string(afterRaw), "不能进入历史的思考正文") {
		t.Fatal("清理改变了历史过程轨或泄露思考正文")
	}
	var snapshot []byte
	if err := db.QueryRow(t.Context(), `SELECT parts FROM messages WHERE id='p6_answer'`).Scan(&snapshot); err != nil || len(snapshot) == 0 {
		t.Fatal("缺少终态快照", err)
	}
	live, err := chats.Messages(t.Context(), owner, "p6_c3", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Messages[len(live.Messages)-1].Parts) == 0 {
		t.Fatal("运行中事件重放丢失")
	}
	w := request(owner, "/api/v1/runs/p6_r1/tool-results/p6_call", "p6_r1", h.ToolResult)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"data":null`) {
		t.Fatalf("到期 data: %d %s", w.Code, w.Body)
	}
	// 历史异常终态没有助手消息时，唯一的可见文本仍在 events，必须拒绝删除。
	if _, err := db.Exec(t.Context(), `UPDATE runs SET ended_at='2025-01-01' WHERE id='p6_r2'; INSERT INTO run_events(run_id,seq,type,payload) VALUES('p6_r2',1,'message.delta','{"delta":"唯一的历史正文"}')`); err != nil {
		t.Fatal(err)
	}
	if err := service.CleanupRetention(t.Context()); err == nil {
		t.Fatal("无助手消息的历史正文被删除")
	}
	var retained bool
	if err := db.QueryRow(t.Context(), `SELECT count(*)=1 FROM run_events WHERE run_id='p6_r2'`).Scan(&retained); err != nil || !retained {
		t.Fatal("异常历史事件未保留", err)
	}
}
