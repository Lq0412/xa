package provider

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestProviderRedactionPaths 将安全边界输入传给实际 HTTP 错误转换和流解析器，不替代模型依赖。
func TestProviderRedactionPaths(t *testing.T) {
	secret := rand.Text()
	m := NewBailian(secret, "")
	for _, text := range []string{
		"Bearer " + secret, "APIKey=" + secret, "密码是 " + secret,
		"person@example.com 13987654321", strings.Repeat("中", 295) + "Bearer " + secret + strings.Repeat("尾", 100),
	} {
		problem := bailianError{Code: "InvalidParameter", Message: text}
		encoded, err := json.Marshal(map[string]any{"error": problem})
		if err != nil {
			t.Fatal(err)
		}
		_, streamErr := m.assemble(t.Context(), strings.NewReader("data: "+string(encoded)+"\n\n"), nil, time.Now(), false)
		for _, failure := range []error{m.providerError(400, problem, false), streamErr, &ProviderError{Kind: ProviderErrorBadRequest, Message: text}} {
			var pe *ProviderError
			if !errors.As(failure, &pe) {
				t.Fatal("未保留 ProviderError")
			}
			var logs bytes.Buffer
			slog.New(slog.NewJSONHandler(&logs, nil)).Error("供应商错误", "error", failure)
			for _, out := range []string{failure.Error(), logs.String()} {
				for _, sensitive := range []string{secret, "person@example.com", "13987654321"} {
					if strings.Contains(out, sensitive) {
						t.Fatal("对外错误或日志泄漏")
					}
				}
			}
			if failure == streamErr || pe.HTTPStatus != 0 {
				if len([]rune(pe.Message)) > 300 || strings.Contains(pe.Message, secret) {
					t.Fatal("适配器 Message 未先脱敏后裁剪")
				}
			}
		}
	}
}
