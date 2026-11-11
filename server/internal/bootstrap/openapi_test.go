package bootstrap

import (
	"net/http"
	"os"
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

// TestOpenAPIRoutes 双向对照实际注册与提交的文档，全部接口均需有约定，不再保留允许清单。
func TestOpenAPIRoutes(t *testing.T) {
	mux := &recordingMux{http.NewServeMux(), map[string]bool{}}
	registerRoutes(mux, nil, documentationHandlers())
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
		if !documented[route] {
			t.Errorf("已注册但缺少文档：%s", route)
		}
	}
	for route := range documented {
		if !mux.patterns[route] {
			t.Errorf("文档包含未注册路由：%s", route)
		}
	}
}
