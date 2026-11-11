package bootstrap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// recordingMux 记录真正传给标准库的模式，避免仅拿文档自身验证文档。
type recordingMux struct {
	*http.ServeMux
	patterns map[string]bool
}

func (m *recordingMux) Handle(pattern string, handler http.Handler) {
	m.patterns[pattern] = true
	m.ServeMux.Handle(pattern, handler)
}

func (m *recordingMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(handler))
}

// TestOpenAPIRoutes 对照实际注册与提交的文档；第一步的未迁路由逐项列明。
func TestOpenAPIRoutes(t *testing.T) {
	mux := &recordingMux{http.NewServeMux(), map[string]bool{}}
	registerRoutes(mux, nil, documentationHandlers())
	if mux.patterns["/health/"] {
		delete(mux.patterns, "/health/")
		// 健康检查仍是嵌套 ServeMux，从它实际的注册代码展开。
		file, err := parser.ParseFile(token.NewFileSet(), "../framework/http/server.go", nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Fatal("健康检查路由不是字符串常量")
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			mux.patterns[pattern] = true
			return true
		})
	}
	data, err := os.ReadFile("../../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, operations := range spec.Paths {
		for method := range operations {
			switch method {
			case "get", "post", "put", "delete", "patch", "head", "options", "trace":
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	for route := range mux.patterns {
		if !documented[route] && !unmigratedRoutes[route] {
			t.Errorf("已注册但缺少文档：%s", route)
		}
	}
	for route := range documented {
		if !mux.patterns[route] {
			t.Errorf("文档包含未注册路由：%s", route)
		}
		if unmigratedRoutes[route] {
			t.Errorf("已迁接口仍在允许清单中：%s", route)
		}
	}
	for route := range unmigratedRoutes {
		if !mux.patterns[route] {
			t.Errorf("允许清单包含未注册路由：%s", route)
		}
	}
}

// unmigratedRoutes 仅允许第一步尚未迁移的 35 个接口，第二步清空。
var unmigratedRoutes = map[string]bool{
	"GET /health/live":                                      true,
	"GET /health/ready":                                     true,
	"GET /api/v1/agent-library":                             true,
	"PUT /api/v1/agent-library/{agent_id}":                  true,
	"GET /api/v1/tools":                                     true,
	"GET /api/v1/agents/{agent_id}/tools":                   true,
	"PUT /api/v1/agents/{agent_id}/tools":                   true,
	"GET /api/v1/memories":                                  true,
	"POST /api/v1/memories":                                 true,
	"PUT /api/v1/memories/{memory_id}":                      true,
	"DELETE /api/v1/memories/{memory_id}":                   true,
	"POST /api/v1/memories/{memory_id}/restore":             true,
	"GET /api/v1/memories/{memory_id}/events":               true,
	"GET /api/v1/agents/{agent_id}/memory":                  true,
	"PUT /api/v1/agents/{agent_id}/memory-policy":           true,
	"GET /api/v1/memory/stats":                              true,
	"POST /api/v1/memory/jobs/{job_id}/retry":               true,
	"POST /api/v1/conversations":                            true,
	"GET /api/v1/conversations":                             true,
	"GET /api/v1/conversations/{conversation_id}/messages":  true,
	"GET /api/v1/conversations/{conversation_id}/summary":   true,
	"POST /api/v1/conversations/{conversation_id}/messages": true,
	"GET /api/v1/runs/{run_id}/events":                      true,
	"POST /api/v1/runs/{run_id}/cancel":                     true,
	"POST /api/v1/approvals/{approval_id}/decision":         true,
	"GET /api/v1/admin/approvals":                           true,
	"GET /api/v1/approvals":                                 true,
	"GET /api/v1/agents/{agent_id}/activity":                true,
	"GET /api/v1/runs":                                      true,
	"GET /api/v1/runs/{run_id}":                             true,
	"GET /api/v1/runs/{run_id}/spans":                       true,
	"GET /api/v1/runs/{run_id}/tool-results/{call_id}":      true,
	"GET /api/v1/admin/runs":                                true,
	"GET /api/v1/admin/runs/{run_id}":                       true,
	"GET /api/v1/admin/runs/{run_id}/spans":                 true,
}
