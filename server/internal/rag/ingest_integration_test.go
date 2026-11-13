//go:build integration

package rag_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/framework/postgres"
	frameworkredis "linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/framework/storage"
	"linq-bs/server/internal/infra/provider"
	"linq-bs/server/internal/rag"
	raghttp "linq-bs/server/internal/rag/transport/http"
	"linq-bs/server/internal/system/user"
)

// TestKnowledgeR1 在独立数据库中使用真实 Redis、S3、百炼和 Huma 接口验收。
func TestKnowledgeR1(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal("集成配置不完整")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	adminDB, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminDB.Close)
	dbName := "kb_test_" + strings.ToLower(rand.Text())
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
	// 仅清理本测试数据库的对象前缀，即使断言失败也不触及其他环境。
	t.Cleanup(func() {
		err := objects.ListBefore(context.Background(), "knowledge/"+dbName+"/", time.Now().Add(time.Minute), func(key string) error { return objects.Delete(context.Background(), key) })
		if err != nil {
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
	resolver := auth.NewRedisResolver(sessions, func(ctx context.Context, id string) (auth.Principal, error) {
		u, e := users.Get(ctx, id)
		if e != nil {
			return auth.Principal{}, e
		}
		return auth.Principal{UserID: u.ID, WorkspaceID: u.WorkspaceID, Roles: []string{u.Role}}, nil
	})
	account := func(role string) (auth.Principal, string) {
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
	_, otherToken := account("user")
	_, adminToken := account("admin")
	svc := &rag.Service{DB: db, Objects: objects, Config: cfg.Knowledge}
	ingestor, err := rag.NewIngestor(svc, provider.NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	api := frameworkhttp.NewAPI(mux)
	(&raghttp.Handler{Knowledge: svc, Config: cfg.Knowledge}).RegisterRoutes(api, resolver)
	server := httptest.NewServer(frameworkhttp.WithRequestID(mux))
	defer server.Close()
	request := func(method, path, token, contentType string, body []byte, want int) []byte {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s: HTTP %d want %d: %s", method, path, res.StatusCode, want, data)
		}
		if want >= 400 {
			var envelope map[string]any
			if json.Unmarshal(data, &envelope) != nil || envelope["code"] == nil || envelope["message"] == nil || envelope["request_id"] == nil || len(envelope) != 3 {
				t.Fatalf("错误体发生变化: %s", data)
			}
		}
		return data
	}
	jsonRequest := func(method, path, token, body string, want int) []byte {
		return request(method, path, token, "application/json", []byte(body), want)
	}
	create := func(token, name, visibility string) rag.Base {
		var b rag.Base
		data := jsonRequest("POST", "/knowledge-bases", token, fmt.Sprintf(`{"name":%q,"visibility":%q}`, name, visibility), 201)
		if err := json.Unmarshal(data, &b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	base := create(ownerToken, "中文资料", "private")
	root := "/knowledge-bases/" + base.ID
	public := create(adminToken, "公共资料", "public")
	t.Run("permissions", func(t *testing.T) {
		jsonRequest("POST", "/knowledge-bases", ownerToken, `{"name":"不能创建公共库","visibility":"public"}`, 403)
		for _, token := range []string{otherToken, adminToken} {
			for _, method := range []string{"GET", "PATCH", "DELETE"} {
				jsonRequest(method, root, token, `{"name":"越权"}`, 404)
			}
			jsonRequest("GET", root+"/documents", token, "", 404)
			data := jsonRequest("GET", "/knowledge-bases", token, "", 200)
			if bytes.Contains(data, []byte(base.ID)) {
				t.Fatal("列表泄露他人私有库")
			}
		}
		jsonRequest("GET", "/knowledge-bases/"+public.ID, ownerToken, "", 200)
		jsonRequest("PATCH", "/knowledge-bases/"+public.ID, ownerToken, `{"description":"越权"}`, 403)
		jsonRequest("DELETE", "/knowledge-bases/"+public.ID, ownerToken, "", 403)
		jsonRequest("PATCH", root, ownerToken, `{"description":"验收资料"}`, 200)
		jsonRequest("GET", "/knowledge-bases?limit=-1", ownerToken, "", 400)
	})
	upload := func(path, token, filename string, data []byte, want int) rag.Document {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, e := form.CreateFormFile("file", filename)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = part.Write(data); e != nil {
			t.Fatal(e)
		}
		if e = form.Close(); e != nil {
			t.Fatal(e)
		}
		response := request("POST", path+"/documents", token, form.FormDataContentType(), body.Bytes(), want)
		var doc rag.Document
		if want < 300 {
			if e = json.Unmarshal(response, &doc); e != nil {
				t.Fatal(e)
			}
		}
		return doc
	}
	data, err := os.ReadFile("testdata/chinese.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := upload(root, ownerToken, "中文.md", data, 201)
	t.Run("deduplicate_and_quotas", func(t *testing.T) {
		duplicate := upload(root, ownerToken, "同内容.md", data, 200)
		if duplicate.ID != doc.ID {
			t.Fatal("重复上传新建文档")
		}
		var jobs int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM kb_ingest_jobs WHERE document_id=$1`, doc.ID).Scan(&jobs); err != nil || jobs != 1 {
			t.Fatalf("重复入队: %d %v", jobs, err)
		}
		limited := *svc
		limited.Config.MaxBasesPerUser = 1
		if _, e := limited.Create(ctx, owner, rag.CreateInput{Name: "超限"}); e == nil {
			t.Fatal("知识库配额未生效")
		}
		limited.Config.MaxDocuments = 1
		if _, _, e := limited.Upload(ctx, owner, base.ID, "超限.txt", []byte("文档数量超限")); e == nil {
			t.Fatal("文档配额未生效")
		}
		limited.Config.MaxDocuments = 100
		limited.Config.MaxUserBytes = int64(len(data))
		if _, _, e := limited.Upload(ctx, owner, base.ID, "超限.txt", []byte("用户空间超限")); e == nil {
			t.Fatal("用户空间配额未生效")
		}
		upload(root, ownerToken, "伪装.pdf", []byte("不是PDF"), 400)
		upload(root, ownerToken, "错误编码.txt", []byte{0xff, 0xfe}, 400)
		upload(root, ownerToken, "超限.txt", bytes.Repeat([]byte("x"), int(cfg.Knowledge.MaxFileBytes)+1), 413)
		var documents int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM kb_documents`).Scan(&documents); e != nil || documents != 1 {
			t.Fatal("配额失败写入了文档")
		}
		objectCount := 0
		if e := objects.ListBefore(ctx, "knowledge/"+dbName+"/", time.Now().Add(time.Minute), func(string) error { objectCount++; return nil }); e != nil || objectCount != 1 {
			t.Fatalf("失败上传遗留对象: %d %v", objectCount, e)
		}
	})
	process := func(w *rag.Ingestor, wantError bool) {
		t.Helper()
		processed, e := w.ProcessNext(ctx)
		if !processed || (e != nil) != wantError {
			t.Fatalf("任务结果 processed=%v err=%v", processed, e)
		}
	}
	t.Run("orphan_cleanup", func(t *testing.T) {
		key := "knowledge/" + dbName + "/orphan"
		if e := objects.Put(ctx, key, strings.NewReader("孤儿文件"), int64(len("孤儿文件")), "text/plain"); e != nil {
			t.Fatal(e)
		}
		cleanup := *svc
		cleanup.Config.JobTimeout = time.Millisecond
		cleanup.Config.OrphanGrace = time.Second
		time.Sleep(1100 * time.Millisecond)
		if e := cleanup.CleanupOrphans(ctx); e != nil {
			t.Fatal(e)
		}
		if object, e := objects.Open(ctx, key); e == nil {
			object.Close()
			t.Fatal("孤儿对象未被清理")
		}
		current, e := svc.GetDocument(ctx, owner, base.ID, doc.ID)
		if e != nil {
			t.Fatal(e)
		}
		var existingKey string
		if e = db.QueryRow(ctx, `SELECT object_key FROM kb_documents WHERE id=$1`, current.ID).Scan(&existingKey); e != nil {
			t.Fatal(e)
		}
		object, e := objects.Open(ctx, existingKey)
		if e != nil {
			t.Fatal("清理误删了有文档引用的原文")
		}
		object.Close()
	})
	process(ingestor, false)
	var ready rag.Document
	if err = json.Unmarshal(jsonRequest("GET", root+"/documents/"+doc.ID, ownerToken, "", 200), &ready); err != nil || ready.Status != "ready" {
		t.Fatalf("Markdown未ready: %+v %v", ready, err)
	}
	chunksPath := root + "/documents/" + doc.ID + "/chunks"
	var page rag.ChunkPage
	if err = json.Unmarshal(jsonRequest("GET", chunksPath, ownerToken, "", 200), &page); err != nil || len(page.Items) == 0 || len(page.Items[0].HeadingPath) == 0 {
		t.Fatal("切片或标题路径缺失")
	}
	var indexed bool
	if err = db.QueryRow(ctx, `SELECT bool_and(search_text<>'' AND fts<>''::tsvector AND vector_dims(embedding)=1024) FROM kb_chunks WHERE document_id=$1`, doc.ID).Scan(&indexed); err != nil || !indexed {
		t.Fatal("分词、全文索引或向量没有保存")
	}
	jsonRequest("POST", root+"/documents/"+doc.ID+"/retry", ownerToken, "", 409)
	t.Run("retry_replaces_chunks", func(t *testing.T) {
		// 从已成功的真实文档发起保留的 rebuild 任务，用真实连接拒绝制造向量化失败。
		if _, e := db.Exec(ctx, `INSERT INTO kb_ingest_jobs(id,workspace_id,document_id,kind,owner_user_id) VALUES($1,$2,$3,'rebuild',$4)`, "rebuild_"+doc.ID, owner.WorkspaceID, doc.ID, owner.UserID); e != nil {
			t.Fatal(e)
		}
		broken, e := rag.NewIngestor(svc, provider.NewBailian(cfg.Bailian.APIKey, "http://127.0.0.1:1"))
		if e != nil {
			t.Fatal(e)
		}
		process(broken, true)
		failure := jsonRequest("GET", root+"/documents/"+doc.ID, ownerToken, "", 200)
		if !bytes.Contains(failure, []byte("embedding_failed")) {
			t.Fatal("缺少失败原因")
		}
		jsonRequest("POST", root+"/documents/"+doc.ID+"/retry", ownerToken, "", 200)
		process(ingestor, false)
		var count, distinct int
		if e := db.QueryRow(ctx, `SELECT count(*),count(DISTINCT chunk_index) FROM kb_chunks WHERE document_id=$1`, doc.ID).Scan(&count, &distinct); e != nil || count != ready.ChunkCount || distinct != count {
			t.Fatalf("重试增加切片: %d/%d want %d: %v", count, distinct, ready.ChunkCount, e)
		}
	})
	t.Run("text_pdf_scan_and_dead", func(t *testing.T) {
		txt := upload(root, ownerToken, "中文.txt", []byte(strings.Repeat("中文文本按段落切片，保留字符范围。\n", 100)), 201)
		process(ingestor, false)
		jsonRequest("GET", root+"/documents/"+txt.ID+"/chunks?limit=1", ownerToken, "", 200)
		textPDF, e := os.ReadFile(os.Getenv("XA_KB_TEST_TEXT_PDF"))
		if e != nil {
			t.Fatal("需设置 XA_KB_TEST_TEXT_PDF 指向真实中文文本 PDF")
		}
		scanPDF, e := os.ReadFile(os.Getenv("XA_KB_TEST_SCAN_PDF"))
		if e != nil {
			t.Fatal("需设置 XA_KB_TEST_SCAN_PDF 指向真实扫描 PDF")
		}
		pdf := upload(root, ownerToken, "中文手册.pdf", textPDF, 201)
		process(ingestor, false)
		var pdfPage rag.ChunkPage
		if e = json.Unmarshal(jsonRequest("GET", root+"/documents/"+pdf.ID+"/chunks", ownerToken, "", 200), &pdfPage); e != nil || len(pdfPage.Items) == 0 || pdfPage.Items[0].PageFrom == nil {
			t.Fatal("PDF缺少页码")
		}
		scan := upload(root, ownerToken, "扫描公报.pdf", scanPDF, 201)
		for i := 0; i < 3; i++ {
			if _, e = db.Exec(ctx, `UPDATE kb_ingest_jobs SET next_run_at=now() WHERE document_id=$1`, scan.ID); e != nil {
				t.Fatal(e)
			}
			process(ingestor, true)
		}
		failure := jsonRequest("GET", root+"/documents/"+scan.ID, ownerToken, "", 200)
		if !bytes.Contains(failure, []byte("扫描件，暂不支持 OCR")) {
			t.Fatal("扫描件错误不明确")
		}
		var status string
		var attempts int
		if e = db.QueryRow(ctx, `SELECT status,attempts FROM kb_ingest_jobs WHERE document_id=$1`, scan.ID).Scan(&status, &attempts); e != nil || status != "dead" || attempts != 3 {
			t.Fatalf("重试未进入dead: %s %d %v", status, attempts, e)
		}
		limited := cfg.Knowledge
		limited.MaxPDFPages = 1
		if _, e = rag.Parse(ctx, textPDF, "application/pdf", limited); e == nil {
			t.Fatal("PDF页数限制未生效，样本应至少两页")
		}
	})
	t.Run("delete_cleanup", func(t *testing.T) {
		jsonRequest("DELETE", root+"/documents/"+doc.ID, ownerToken, "", 204)
		jsonRequest("GET", root+"/documents/"+doc.ID, ownerToken, "", 404)
		jsonRequest("GET", chunksPath, ownerToken, "", 404)
		process(ingestor, false)
		var n int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM kb_chunks WHERE document_id=$1`, doc.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("删除没有清理切片")
		}
		var key string
		// HTTP 不返回 object_key，删除前已知本次原文应在 S3 消失，以原文哈希核对文档行已清除。
		if e := db.QueryRow(ctx, `SELECT object_key FROM kb_documents WHERE id=$1`, doc.ID).Scan(&key); e != pgx.ErrNoRows {
			t.Fatal("文档行没有物理清理")
		}
		jsonRequest("DELETE", root, ownerToken, "", 204)
		jsonRequest("GET", root, ownerToken, "", 404)
		for {
			processed, e := ingestor.ProcessNext(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if !processed {
				break
			}
		}
		if e := db.QueryRow(ctx, `SELECT count(*) FROM kb_documents WHERE knowledge_base_id=$1`, base.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("删除库没有清理文档")
		}
		remaining := 0
		if e := objects.ListBefore(ctx, "knowledge/"+dbName+"/", time.Now().Add(time.Minute), func(string) error { remaining++; return nil }); e != nil || remaining != 0 {
			t.Fatalf("S3仍有对象 %d %v", remaining, e)
		}
		var completed int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM kb_ingest_jobs WHERE kind='delete' AND status='completed'`).Scan(&completed); e != nil || completed < 4 {
			t.Fatal("缺少删除完成证据")
		}
	})
	t.Run("concurrent_duplicate", func(t *testing.T) {
		b := create(ownerToken, "并发资料", "private")
		var group sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			group.Go(func() {
				_, _, e := svc.Upload(ctx, owner, b.ID, "并发.txt", []byte("并发上传的同一个真实文件"))
				results <- e
			})
		}
		group.Wait()
		close(results)
		for e := range results {
			if e != nil {
				t.Fatal(e)
			}
		}
		var n int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM kb_documents WHERE knowledge_base_id=$1`, b.ID).Scan(&n); e != nil || n != 1 {
			t.Fatal("并发重复上传失效")
		}
		if e := svc.Delete(ctx, owner, b.ID); e != nil {
			t.Fatal(e)
		}
		for {
			processed, e := ingestor.ProcessNext(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if !processed {
				break
			}
		}
	})
	t.Run("worker_process", func(t *testing.T) {
		b := create(ownerToken, "worker资料", "private")
		d := upload("/knowledge-bases/"+b.ID, ownerToken, "后台.txt", []byte("现有 worker 进程处理知识摄取任务，与记忆循环一起运行。"), 201)
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		cfg.HTTP.WorkerAddress = listener.Addr().String()
		listener.Close()
		workerCtx, stop := context.WithCancel(ctx)
		defer stop()
		done := make(chan error, 1)
		go func() {
			done <- bootstrap.NewWorker(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Run(workerCtx)
		}()
		deadline := time.After(30 * time.Second)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case e := <-done:
				t.Fatalf("worker提前结束: %v", e)
			case <-deadline:
				t.Fatal("worker未完成摄取")
			case <-ticker.C:
				current, e := svc.GetDocument(ctx, owner, b.ID, d.ID)
				if e != nil {
					t.Fatal(e)
				}
				if current.Status == "ready" {
					stop()
					if e = <-done; e != nil {
						t.Fatal(e)
					}
					t.Log("真实 worker：文档 ready，取消后进程正常退出")
					return
				}
			}
		}
	})
	t.Log("R1：权限、配额、去重、重试、中文解析、PDF页码、扫描件dead、切片查询、异步删除及真实worker通过")
}
