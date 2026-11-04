package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/workspace"
)

type requestIDKey struct{}

// WithRequestID 为每个请求补齐可追踪标识；错误响应会原样返回该标识。
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 100 {
			requestID = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// Authenticate 将 Bearer 令牌转换为上下文主体；路由处理器只从 Resolver 获取身份。
func Authenticate(resolver auth.Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
			return
		}
		principal, err := resolver.Resolve(auth.WithSessionToken(r.Context(), token))
		if err != nil {
			status := http.StatusUnauthorized
			if !errors.Is(err, auth.ErrUnauthenticated) {
				status = http.StatusServiceUnavailable
			}
			WriteError(w, r, status, "unauthenticated", err)
			return
		}
		ctx := auth.WithPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalPrincipal 允许公共 Agent 浏览接口在没有登录时使用默认工作空间。
func OptionalPrincipal(resolver auth.Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{WorkspaceID: workspace.DefaultWorkspaceID})))
			return
		}
		principal, err := resolver.Resolve(auth.WithSessionToken(r.Context(), token))
		if err != nil {
			WriteError(w, r, http.StatusUnauthorized, "unauthenticated", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func Principal(ctx context.Context) (auth.Principal, bool) {
	principal, ok := auth.PrincipalFromContext(ctx)
	return principal, ok
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	message := err.Error()
	if status >= 500 {
		message = "服务暂时不可用"
	}
	WriteJSON(w, status, map[string]string{"code": code, "message": message, "request_id": RequestID(r.Context())})
}
