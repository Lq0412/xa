package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/runtime"
)

// 使用真实校验和 HTTP 错误映射，确认过长消息不会变成成功的 SSE 响应。
func TestMessageTooLongHTTP(t *testing.T) {
	err := runtime.ValidateCurrent(strings.Repeat("字", runtime.ModelContextTokens+1))
	w := httptest.NewRecorder()
	conversationError(w, httptest.NewRequest("POST", "/api/v1/conversations/test/messages", nil), err)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `"code":"message_too_long"`) || !strings.Contains(w.Body.String(), "请缩短或分段发送") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
