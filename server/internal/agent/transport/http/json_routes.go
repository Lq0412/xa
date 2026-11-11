package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// routeOutput 让 Huma 为各接口的 JSON 响应生成对应的 Schema。
type routeOutput[T any] struct{ Body T }

// protectedAgentOperations 为 Agent 相关接口复用认证和错误文档。
func protectedAgentOperations(api huma.API, resolver auth.Resolver, tag, unavailable string) func(string, string, string, string, int, int64, ...int) huma.Operation {
	protect := func(next http.Handler) http.Handler {
		return frameworkhttp.Authenticate(resolver, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := requirePrincipal(w, r); ok {
				next.ServeHTTP(w, r)
			}
		}))
	}
	descriptions := map[int]string{
		400: "请求 JSON 无效（invalid_json）", 401: "会话无效（unauthenticated）",
		403: "没有资源操作权限（agent_forbidden）", 404: "资源不存在（agent_not_found）",
		409: "版本冲突（agent_conflict）", 422: "请求数据无效（invalid_agent）",
		503: unavailable,
	}
	return func(id, method, path, summary string, status int, bodyLimit int64, codes ...int) huma.Operation {
		responses := make(map[int]string, len(codes))
		for _, code := range codes {
			responses[code] = descriptions[code]
		}
		return frameworkhttp.OperationAuth(frameworkhttp.Operation(api, tag, id, method, path, summary, status, bodyLimit, responses), protect, false)
	}
}

// writeLegacyJSON 保留直接调用的入口，响应内容仍由类型化处理器构造。
func writeLegacyJSON[T any](w http.ResponseWriter, status int, out *routeOutput[T], err error) {
	if err != nil {
		frameworkhttp.WriteAPIError(w, err.(huma.StatusError))
		return
	}
	frameworkhttp.WriteJSON(w, status, out.Body)
}

func serveLegacyJSON[I, T any](w http.ResponseWriter, r *http.Request, status int, input *I, handler func(context.Context, *I) (*routeOutput[T], error)) {
	if _, ok := requirePrincipal(w, r); !ok {
		return
	}
	out, err := handler(r.Context(), input)
	writeLegacyJSON(w, status, out, err)
}

func serveLegacyNoContent[I any](w http.ResponseWriter, r *http.Request, input *I, handler func(context.Context, *I) (*frameworkhttp.NoContent, error)) {
	if _, ok := requirePrincipal(w, r); !ok {
		return
	}
	_, err := handler(r.Context(), input)
	writeLegacyNoContent(w, err)
}

func writeLegacyNoContent(w http.ResponseWriter, err error) {
	if err != nil {
		frameworkhttp.WriteAPIError(w, err.(huma.StatusError))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
