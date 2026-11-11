//go:build integration

package bootstrap_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"linq-bs/server/internal/agent/conversation"
	"linq-bs/server/internal/agent/run"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
)

// TestP7IdleReconnect 验证真实数据库中已追平的审批流立即响应，越权请求不会提前进入 SSE。
func TestP7IdleReconnect(t *testing.T) {
	db := newP5Database(t)
	seedP6Queries(t, db)
	runs := &run.RunService{DB: db, Store: run.NewPGStore(db)}
	handler := &agenthttp.RunHandler{Runs: runs, Conversations: &conversation.Service{DB: db, Runs: runs}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runs/{run_id}/events", func(w http.ResponseWriter, r *http.Request) {
		p := auth.Principal{UserID: "p6_u1", WorkspaceID: "default"}
		handler.Events(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, id := range []string{"p6_live", "p6_cross", "missing"} {
		t.Run(id, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/runs/"+id+"/events?after=1", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("订阅未立即响应: %v", err)
			}
			defer response.Body.Close()
			if id != "p6_live" {
				if response.StatusCode != http.StatusNotFound || strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
					t.Fatalf("权限检查必须先于 SSE: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
				}
				return
			}
			if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
				t.Fatalf("订阅类型错误: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
			}
			line, err := bufio.NewReader(response.Body).ReadString('\n')
			if err != nil || line != ": ping\n" {
				t.Fatalf("空闲订阅缺少连接确认: %q %v", line, err)
			}
		})
	}
}
