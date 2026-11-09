//go:build integration

package provider

import (
	"bytes"
	"crypto/rand"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestBailianRealHTTPErrorSafety 使用真实供应商拒绝未知模型，验证 P1 HTTP 错误和日志路径。
func TestBailianRealHTTPErrorSafety(t *testing.T) {
	key, base := os.Getenv("XA_BAILIAN_API_KEY"), os.Getenv("XA_BAILIAN_BASE_URL")
	if key == "" || base == "" {
		t.Fatal("缺少百炼配置")
	}
	m := NewBailian(key, base)
	_, err := m.Chat(t.Context(), ChatRequest{Model: "r1-invalid-" + strings.ToLower(rand.Text()), Messages: []Message{{Role: "user", Content: "验证错误分类"}}}, nil)
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.HTTPStatus < 400 {
		t.Fatal("真实供应商未拒绝无效模型")
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Error("真实供应商错误", "error", pe)
	if strings.Contains(pe.Message, key) || strings.Contains(pe.Error(), key) || strings.Contains(logs.String(), key) || len([]rune(pe.Message)) > 300 {
		t.Fatal("真实错误路径泄漏或超长")
	}
	t.Logf("真实 HTTP 错误安全检查通过，status=%d kind=%s", pe.HTTPStatus, pe.Kind)
}
