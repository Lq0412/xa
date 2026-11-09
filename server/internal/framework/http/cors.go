package http

import (
	"net/http"
	"strings"
)

// WithAllowOrigins 在开发/E2E 场景允许浏览器跨域访问 API（含 SSE）。
func WithAllowOrigins(origins []string, next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, ok := allowed[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
			w.Header().Set(
				"Access-Control-Allow-Headers",
				"Authorization, Content-Type, X-XA-Stream, Idempotency-Key, Last-Event-ID",
			)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions && origin != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
