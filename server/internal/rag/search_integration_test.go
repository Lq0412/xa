//go:build integration

package rag_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/framework/postgres"
	frameworkredis "linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/framework/storage"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/rag"
	"linq-bs/server/internal/system/user"
)

// TestKnowledgeR2 使用独立数据库、真实 S3/百炼和生产 server 验证检索授权及工具循环。
func TestKnowledgeR2(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal("集成配置不完整")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	adminDB, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminDB.Close)
	dbName := "kb_r2_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{dbName}.Sanitize()
	if _, err = adminDB.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.Exec(context.Background(), "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	cfg.Postgres.Database = dbName
	cfg.Admin.Email, cfg.Admin.Password = "", ""
	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err = db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	if err = postgres.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	objects, err := storage.Open(ctx, cfg.ObjectStore)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(objects.Close)
	t.Cleanup(func() {
		if err := objects.ListBefore(context.Background(), "knowledge/"+dbName+"/", time.Now().Add(time.Minute), func(key string) error { return objects.Delete(context.Background(), key) }); err != nil {
			t.Error(err)
		}
	})
	cache, err := frameworkredis.Open(ctx, cfg.Redis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	sessions := auth.NewRedisSessionStore(cache, time.Hour)
	users := user.NewService(user.NewPGRepository(db))
	account := func(role string) (auth.Principal, string) {
		t.Helper()
		u, e := users.Register(ctx, user.Credentials{Email: strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, `UPDATE users SET role=$2 WHERE id=$1`, u.ID, role); e != nil {
			t.Fatal(e)
		}
		p := auth.Principal{UserID: u.ID, WorkspaceID: u.WorkspaceID, Roles: []string{role}}
		token, e := sessions.Create(ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { sessions.Delete(context.Background(), token) })
		return p, token
	}
	owner, ownerToken := account("user")
	other, otherToken := account("user")
	admin, adminToken := account("admin")
	model := provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)
	svc := &rag.Service{DB: db, Objects: objects, Config: cfg.Knowledge}
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	agents.KnowledgeValidator = svc.ValidateBindings
	search, err := rag.NewSearcher(svc, agents, model)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := rag.NewIngestor(svc, model)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTP.ServerAddress = listener.Addr().String()
	listener.Close()
	serverCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- bootstrap.NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Run(serverCtx)
	}()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	client := &http.Client{Timeout: 90 * time.Second}
	url := "http://" + cfg.HTTP.ServerAddress
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, e := client.Get(url + "/health/ready")
		if e == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("验收 server 未就绪")
		}
		time.Sleep(100 * time.Millisecond)
	}
	request := func(method, path, token string, body any, want int) []byte {
		t.Helper()
		data, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		req, e := http.NewRequestWithContext(ctx, method, url+"/api/v1"+path, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		data, e = io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s: HTTP %d want %d: %s", method, path, resp.StatusCode, want, data)
		}
		if want >= 400 {
			var envelope map[string]any
			if json.Unmarshal(data, &envelope) != nil || len(envelope) != 3 || envelope["code"] == nil || envelope["message"] == nil || envelope["request_id"] == nil {
				t.Fatalf("错误体发生变化: %s", data)
			}
		}
		return data
	}
	createBase := func(token, name, visibility string) rag.Base {
		t.Helper()
		var b rag.Base
		if err := json.Unmarshal(request("POST", "/knowledge-bases", token, rag.CreateInput{Name: name, Visibility: visibility}, 201), &b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	base := createBase(ownerToken, "青岚办理手册", "private")
	foreign := createBase(otherToken, "他人资料", "private")
	public := createBase(adminToken, "公共须知", "public")
	adminPrivate := createBase(adminToken, "管理员私有资料", "private")
	upload := func(p auth.Principal, b rag.Base, text string) rag.Document {
		t.Helper()
		d, _, e := svc.Upload(ctx, p, b.ID, "手册.md", []byte(text))
		if e != nil {
			t.Fatal(e)
		}
		if ok, e := ingestor.ProcessNext(ctx); !ok || e != nil {
			t.Fatal("真实摄取失败", e)
		}
		return d
	}
	doc := upload(owner, base, "# 青岚设备办理手册\n\n## 退货期限\n\n青岚设备的退货受理编号为 QX-482。客户签收之日起十四天内可以申请退货。请保留原包装和完整附件。超过十四天请联系售后处理维修。\n\n## 发票办理\n\n电子发票在付款后三个工作日内开具，可在账户订单记录中下载。发票抬头必须与实际购买单位一致。")
	upload(other, foreign, "# 他人私有手册\n\n青岚设备 QX-482 的机密处理口令为紫色海鸥，只能由私有资料所有者查看。")
	publicDoc := upload(admin, public, "# 公共营业须知\n\n云杉服务中心每周一至周五开放，营业时间为上午九点至下午五点，周末不对外营业。")
	bind := func(id string) []binding.KnowledgeBinding {
		return []binding.KnowledgeBinding{{KnowledgeBaseID: id, Enabled: true}}
	}
	configuration := func(knowledge []binding.KnowledgeBinding) map[string]any {
		return map[string]any{"model": cfg.LightModel, "enable_thinking": false, "memory_policy": map[string]any{"enabled": false}, "knowledge": knowledge,
			"soul_md": "你是资料问答助手。涉及资料的问题先调用 kb_search，根据工具返回的原文回答。没有命中时说明资料不足，不编造内容或来源。",
			"tools":   []binding.ToolBinding{{Name: "current_time", Enabled: false}},
		}
	}
	createAgent := func(token, visibility string, knowledge []binding.KnowledgeBinding, want int) definition.AgentDefinition {
		t.Helper()
		data := request("POST", "/agents", token, map[string]any{"name": "知识检索验收 " + rand.Text()[:8], "visibility": visibility, "configuration": configuration(knowledge)}, want)
		if want != 201 {
			return definition.AgentDefinition{}
		}
		var result struct {
			Agent definition.AgentDefinition `json:"agent"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result.Agent
	}
	t.Run("保存绑定权限", func(t *testing.T) {
		createAgent(otherToken, "private", bind(base.ID), 422)
		createAgent(adminToken, "private", bind(base.ID), 422)
		createAgent(ownerToken, "private", bind(public.ID), 422)
		createAgent(adminToken, "public", bind(adminPrivate.ID), 422)
		createAgent(ownerToken, "private", append(bind(base.ID), bind(base.ID)...), 422)
		filtered := bind(base.ID)
		filtered[0].Filter = []byte(`{"x":1}`)
		createAgent(ownerToken, "private", filtered, 422)
		request("GET", "/knowledge-bases/"+base.ID, adminToken, nil, 404)
	})
	a := createAgent(ownerToken, "private", bind(base.ID), 201)
	pubAgent := createAgent(adminToken, "public", bind(public.ID), 201)
	unbound := createAgent(ownerToken, "private", nil, 201)
	lookup := func(p auth.Principal, agent definition.AgentDefinition, in rag.SearchInput) rag.SearchResult {
		t.Helper()
		out, e := search.Search(ctx, p, agent.ID, agent.CurrentVersionID, in)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	t.Run("混合检索与出处", func(t *testing.T) {
		for _, q := range []string{"QX-482", "青岚设备退货期限", "买到的东西不用了，收到以后最晚多少天能申请退回？"} {
			out := lookup(owner, a, rag.SearchInput{Query: q, TopK: 1})
			if len(out.Items) != 1 || !strings.Contains(out.Items[0].Content, "十四天") {
				t.Fatalf("未召回退货说明，query=%s: %+v", q, out.Items)
			}
			hit := out.Items[0]
			if hit.DocumentID != doc.ID || hit.SourceID == "" || len(hit.HeadingPath) != 2 || hit.CharEnd-hit.CharStart != utf8.RuneCountInString(hit.Content) || hit.ContentSHA256 == "" || hit.ChunkerVersion == "" {
				t.Fatalf("出处不完整: %+v", hit)
			}
			if q == "QX-482" && (hit.VectorRank == 0 || hit.KeywordRank == 0) {
				t.Fatalf("两路没有共同召回编号: %+v", hit)
			}
			t.Logf("%s：命中退货说明，vector_rank=%d keyword_rank=%d RRF=%.5f", q, hit.VectorRank, hit.KeywordRank, hit.Score)
		}
	})
	t.Run("交集权限与无命中", func(t *testing.T) {
		if out := lookup(owner, a, rag.SearchInput{Query: "QX-482", CollectionIDs: []string{foreign.ID, public.ID}}); len(out.Items) != 0 {
			t.Fatal("参数扩大了绑定范围")
		}
		if out := lookup(owner, unbound, rag.SearchInput{Query: "QX-482"}); len(out.Items) != 0 {
			t.Fatal("无绑定仍返回资料")
		}
		for _, actor := range []auth.Principal{other, admin} {
			if _, err := search.Search(ctx, actor, a.ID, a.CurrentVersionID, rag.SearchInput{Query: "QX-482"}); !errors.Is(err, definition.ErrNotFound) {
				t.Fatal("越权读取私有 Agent", err)
			}
		}
		for _, actor := range []auth.Principal{owner, other} {
			out := lookup(actor, pubAgent, rag.SearchInput{Query: "云杉营业时间"})
			if len(out.Items) == 0 || out.Items[0].DocumentID != publicDoc.ID {
				t.Fatal("公共 Agent 不能使用公共资料")
			}
		}
		out := lookup(owner, a, rag.SearchInput{Query: "spectroheliograph"})
		if len(out.Items) != 0 {
			t.Fatalf("无关查询未被阈值排除: %+v", out.Items)
		}
	})
	t.Run("并发检索", func(t *testing.T) {
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				out, e := search.Search(ctx, owner, a.ID, a.CurrentVersionID, rag.SearchInput{Query: "QX-482"})
				if e != nil || len(out.Items) == 0 {
					t.Errorf("并发查询失败: %v", e)
				}
			})
		}
		wg.Wait()
	})
	t.Run("历史绑定不能授予权限", func(t *testing.T) {
		// R1 允许写入尚未校验的绑定；直接通过仓储准备这种旧版本，验证检索仍独立限制权限。
		for _, tc := range []struct {
			actor              auth.Principal
			token              string
			visibility         string
			allowed, forbidden rag.Base
		}{
			{owner, ownerToken, "private", base, foreign},
			{admin, adminToken, "private", adminPrivate, base},
			{admin, adminToken, "public", public, base},
		} {
			legacy := createAgent(tc.token, tc.visibility, bind(tc.allowed.ID), 201)
			v, e := agents.GetVersion(ctx, tc.actor, legacy.ID, legacy.CurrentVersionID)
			if e != nil {
				t.Fatal(e)
			}
			v.ID += "_legacy"
			v.VersionNumber++
			v.Configuration.Knowledge = append(v.Configuration.Knowledge, bind(tc.forbidden.ID)...)
			if e = definition.NewPGRepository(db).SaveRevision(ctx, tc.actor, definition.Saved{Definition: legacy, Version: v}, legacy.CurrentVersionID); e != nil {
				t.Fatal(e)
			}
			legacy.CurrentVersionID = v.ID
			actor := tc.actor
			if tc.visibility == "public" {
				actor = owner
			}
			out := lookup(actor, legacy, rag.SearchInput{Query: "QX-482 青岚设备"})
			for _, hit := range out.Items {
				if hit.KnowledgeBaseID != tc.allowed.ID {
					t.Fatalf("历史绑定绕过权限: %+v", hit)
				}
			}
		}
	})
	t.Run("片段预算", func(t *testing.T) {
		original := svc.Config.SearchMaxChars
		svc.Config.SearchMaxChars = 20
		defer func() { svc.Config.SearchMaxChars = original }()
		out := lookup(owner, a, rag.SearchInput{Query: "QX-482"})
		if len(out.Items) != 1 || !out.Items[0].Truncated || utf8.RuneCountInString(out.Items[0].Content) != 20 || out.Items[0].CharEnd-out.Items[0].CharStart != 20 {
			t.Fatalf("预算或字符范围无效: %+v", out.Items)
		}
	})
	var runID, callID string
	var snapshot []byte
	t.Run("真实工具循环与快照", func(t *testing.T) {
		var conv struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request("POST", "/conversations", ownerToken, map[string]string{"agent_id": a.ID}, 201), &conv); err != nil {
			t.Fatal(err)
		}
		response := request("POST", "/conversations/"+conv.ID+"/messages", ownerToken, map[string]string{"content": "请查阅知识库，告诉我青岚设备 QX-482 签收后多少天内可以退货？只根据资料回答。"}, 200)
		if !bytes.Contains(response, []byte("event: done")) || (!bytes.Contains(response, []byte("十四")) && !bytes.Contains(response, []byte("14"))) {
			t.Fatalf("真实 Agent 没有给出资料中的答案: %s", response)
		}
		if err := db.QueryRow(ctx, `SELECT run_id,call_id,data FROM tool_results WHERE conversation_id=$1 AND tool_name='kb_search' AND status='succeeded' ORDER BY step,call_index LIMIT 1`, conv.ID).Scan(&runID, &callID, &snapshot); err != nil {
			t.Fatal("没有持久化的检索工具结果", err)
		}
		var stored rag.SearchResult
		if err := json.Unmarshal(snapshot, &stored); err != nil || stored.AgentVersionID != a.CurrentVersionID || len(stored.Items) == 0 || stored.Items[0].DocumentID != doc.ID {
			t.Fatal("命中快照缺失或版本不符", err)
		}
		request("GET", "/runs/"+runID+"/tool-results/"+callID, ownerToken, nil, 200)
		request("GET", "/runs/"+runID+"/tool-results/"+callID, otherToken, nil, 404)
		t.Log("真实百炼工具循环完成：答案包含十四天，运行记录保留原文、标题、出处和版本")
	})
	t.Run("绑定版本不可变", func(t *testing.T) {
		oldVersion := a.CurrentVersionID
		request("PUT", "/agents/"+a.ID+"/config", ownerToken, map[string]any{"name": a.Name, "visibility": "private", "expected_version_id": oldVersion, "configuration": configuration(nil)}, 200)
		updated, e := agents.Get(ctx, owner, a.ID)
		if e != nil {
			t.Fatal(e)
		}
		if out := lookup(owner, updated, rag.SearchInput{Query: "QX-482"}); len(out.Items) != 0 {
			t.Fatal("新版本仍使用旧绑定")
		}
		if out := lookup(owner, a, rag.SearchInput{Query: "QX-482"}); len(out.Items) == 0 {
			t.Fatal("旧运行的不可变版本被改写")
		}
	})
	t.Run("公共Agent的v2工具循环", func(t *testing.T) {
		var conv struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request("POST", "/conversations", otherToken, map[string]string{"agent_id": pubAgent.ID}, 201), &conv); err != nil {
			t.Fatal(err)
		}
		response := request("POST", "/conversations/"+conv.ID+"/messages?stream=2", otherToken, map[string]string{"content": "请检索知识库，查询云杉服务中心周末是否营业，几点到几点开放？"}, 200)
		if !bytes.Contains(response, []byte("run.completed")) || !bytes.Contains(response, []byte("tool.completed")) {
			t.Fatalf("v2 未完成检索工具循环: %s", response)
		}
		var data []byte
		if err := db.QueryRow(ctx, `SELECT data FROM tool_results WHERE conversation_id=$1 AND tool_name='kb_search' AND status='succeeded' ORDER BY step,call_index LIMIT 1`, conv.ID).Scan(&data); err != nil {
			t.Fatal(err)
		}
		var out rag.SearchResult
		if err := json.Unmarshal(data, &out); err != nil || len(out.Items) == 0 || out.Items[0].DocumentID != publicDoc.ID {
			t.Fatal("公共 Agent 命中快照无效", err)
		}
	})
	t.Run("删除立即排除并保留历史", func(t *testing.T) {
		request("DELETE", "/knowledge-bases/"+base.ID+"/documents/"+doc.ID, ownerToken, nil, 204)
		if out := lookup(owner, a, rag.SearchInput{Query: "QX-482"}); len(out.Items) != 0 {
			t.Fatal("删除文档仍被检索")
		}
		if ok, e := ingestor.ProcessNext(ctx); !ok || e != nil {
			t.Fatal("文档清理失败", e)
		}
		request("DELETE", "/knowledge-bases/"+public.ID, adminToken, nil, 204)
		if out := lookup(other, pubAgent, rag.SearchInput{Query: "云杉营业时间"}); len(out.Items) != 0 {
			t.Fatal("删除知识库仍被检索")
		}
		if ok, e := ingestor.ProcessNext(ctx); !ok || e != nil {
			t.Fatal("公共库清理失败", e)
		}
		if runID != "" {
			var after []byte
			if err := db.QueryRow(ctx, `SELECT data FROM tool_results WHERE run_id=$1 AND call_id=$2`, runID, callID).Scan(&after); err != nil || !bytes.Equal(after, snapshot) {
				t.Fatal("历史快照随删除改变", err)
			}
			data := request("GET", "/runs/"+runID+"/tool-results/"+callID, ownerToken, nil, 200)
			if !bytes.Contains(data, []byte("十四天")) {
				t.Fatal("清理后无法核对历史命中原文")
			}
		}
	})
	t.Log(fmt.Sprintf("R2 验收：真实 PostgreSQL、RustFS、%s 与 %s；权限、RRF、工具调用、版本、预算和删除快照完成", provider.EmbeddingModel, cfg.LightModel))
}
