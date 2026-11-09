package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
)

// TestToolValidationHTTP422 通过真实 HTTP 服务器、管理 handler、Service 和 Registry 验证投影。
func TestToolValidationHTTP422(t *testing.T) {
	s := definition.NewService(nil)
	s.ToolValidator = tool.NewRegistry().ValidateBindings
	h := &ManagementHandler{Agents: s}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Create(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{UserID: "user", WorkspaceID: "default"})))
	}))
	defer server.Close()
	for _, name := range []string{"missing", "authorization: Bearer private-value"} {
		resp, err := server.Client().Post(server.URL, "application/json", strings.NewReader(`{"name":"配置校验","visibility":"private","configuration":{"tools":[{"name":"`+name+`","enabled":true}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		var body strings.Builder
		_, err = io.Copy(&body, resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 422 || !strings.Contains(body.String(), "name") || !strings.Contains(body.String(), "invalid_agent") || strings.Contains(body.String(), "private-value") {
			t.Fatal("HTTP 422 字段投影或脱敏失败")
		}
	}
}
