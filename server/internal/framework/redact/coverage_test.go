package redact

import (
	"strings"
	"testing"
)

func TestTextAndStringSensitiveBoundaries(t *testing.T) {
	for _, input := range []string{
		"Bearer secret-value", "authorization: Bearer secret-value", "APIKey=secret-value", "api_key=secret-value", `{"api-key":"secret-value"}`,
		"密码是 secret-value", "password: secret-value with spaces", "邮箱 person@example.com", "电话 13987654321", "+86 13987654321",
		"银行卡 6222029876543210987", "身份证 11010119900101123X", "11010119900101123X", "sk-secret-value",
	} {
		t.Run(strings.Split(input, " ")[0], func(t *testing.T) {
			for _, got := range []string{Text(input), String(input)} {
				if got == input || strings.Contains(got, "secret-value") || strings.Contains(got, "person@example.com") || strings.Contains(got, "13987654321") || strings.Contains(got, "11010119900101123X") {
					t.Fatal("敏感内容未完整脱敏")
				}
			}
			if Text(input) != String(input) {
				t.Fatal("入口规则不一致")
			}
		})
	}
	for _, input := range []string{"普通错误 timeout_ms", "9876543210", "模型 qwen3.8-flash", "name 未注册"} {
		if Text(input) != input {
			t.Fatal("普通内容被改变")
		}
	}
}
