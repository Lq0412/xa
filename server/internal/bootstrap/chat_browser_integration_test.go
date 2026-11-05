//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/prompt"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/system/user"
)

// TestChatBrowser 将真实生产前端代理到独立的低成本模型 server；不替换接口或模型响应。
func TestChatBrowser(t *testing.T) {
	if os.Getenv("XA_BROWSER_INTEGRATION") != "1" {
		t.Skip("使用 XA_BROWSER_INTEGRATION=1 make integration 启用生产前端浏览器验收")
	}
	cfg := loadConfig(t)
	cfg.ChatModel = "qwen3.8-flash"
	cfg.HTTP.ServerAddress = freeAddress(t)
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(root, "web-user", "dist")
	build := exec.CommandContext(t.Context(), "npm", "run", "build")
	build.Dir = filepath.Join(root, "web-user")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Fatal(err)
	}
	db, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	publicID := ""
	defer db.Close()
	password := rand.Text()
	account, err := user.NewService(user.NewPGRepository(db)).Register(t.Context(), user.Credentials{
		Email: "chat-" + strings.ToLower(rand.Text()) + "@example.com", Password: password, DisplayName: "对话验收",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 仅清理本次注册账号及其资源，外键要求先断开 Agent 当前版本，再删除版本和定义。
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, query := range []string{
			`DELETE FROM conversations WHERE user_id=$1`,
			`UPDATE agent_definitions SET current_version_id=NULL WHERE owner_id=$1 OR id=$2`,
			`DELETE FROM agent_versions WHERE agent_id IN (SELECT id FROM agent_definitions WHERE owner_id=$1 OR id=$2)`,
			`DELETE FROM agent_definitions WHERE owner_id=$1 OR id=$2`,
			`DELETE FROM users WHERE id=$1`,
		} {
			args := []any{account.ID}
			if strings.Contains(query, "$2") {
				args = append(args, publicID)
			}
			if _, err := db.Exec(ctx, query, args...); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	// 通过真实领域服务创建独立测试数据，主 Agent 标记直接验证已有持久字段。
	agents := definition.NewService(definition.NewPGRepository(db))
	primary, err := agents.Save(t.Context(), auth.Principal{UserID: account.ID, WorkspaceID: account.WorkspaceID}, definition.SaveInput{
		Name: "我的主 Agent", Visibility: definition.VisibilityPrivate,
		Configuration: definition.Configuration{Prompts: []prompt.Slot{{Name: "system", Content: "请按用户要求用中文回答。", Enabled: true, Scope: "agent"}, {Name: "context", Content: "保留这个未启用片段", Enabled: false, Order: 1, Scope: "agent"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE agent_definitions SET is_default=true WHERE id=$1`, primary.Definition.ID); err != nil {
		t.Fatal(err)
	}
	admin, err := user.NewService(user.NewPGRepository(db)).Authenticate(t.Context(), cfg.Admin.Email, cfg.Admin.Password)
	if err != nil {
		t.Fatal(err)
	}
	public, err := agents.Save(t.Context(), auth.Principal{UserID: admin.ID, WorkspaceID: admin.WorkspaceID, Roles: []string{admin.Role}}, definition.SaveInput{
		Name: "公共布局验收 " + rand.Text(), Visibility: definition.VisibilityPublic, Description: "真实公共 Agent 添加验证",
		Configuration: definition.Configuration{Prompts: []prompt.Slot{{Name: "system", Content: "你是公共助手。", Enabled: true, Scope: "agent"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	publicID = public.Definition.ID
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- bootstrap.NewServer(cfg, slog.Default()).Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(cfg.ShutdownTimeout + time.Second):
			t.Error("测试 server 未按时退出")
		}
	}()
	endpoint := "http://" + cfg.HTTP.ServerAddress
	waitReady(t, endpoint, done)
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	files := http.FileServer(http.Dir(dist))
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			proxy.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(r.URL.Path) == "" {
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer frontend.Close()
	command := exec.CommandContext(ctx, "npm", "test")
	command.Dir = filepath.Join(root, "web-user")
	command.Env = append(os.Environ(), "XA_CHAT_INTEGRATION=1", "XA_WEB_URL="+frontend.URL,
		"XA_E2E_EMAIL="+account.Email, "XA_E2E_PASSWORD="+password,
		"XA_E2E_PRIMARY_ID="+primary.Definition.ID, "XA_E2E_PUBLIC_ID="+publicID, "XA_E2E_PUBLIC_NAME="+public.Definition.Name)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal("对话浏览器验收失败: ", err)
	}
}
