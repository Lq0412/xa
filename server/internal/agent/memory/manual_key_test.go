package memory

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestGeneratedSubjectKey(t *testing.T) {
	for _, key := range []string{"favorite.food", "a", "a" + strings.Repeat("b", 63)} {
		raw, err := json.Marshal(map[string]string{"subject_key": key})
		if err != nil {
			t.Fatal(err)
		}
		if got := generatedSubjectKey(string(raw), nil); got != key {
			t.Fatal(got)
		}
		if got := generatedSubjectKey(string(raw), errors.New("generation failed")); got != "" {
			t.Fatal(got)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"subject_key":null}`, `{"subject_key":1}`, `{"subject_key":"大写"}`, `{"subject_key":"A"}`, `{"subject_key":"a b"}`, `{"subject_key":"_a"}`, `{"subject_key":"` + strings.Repeat("a", 65) + `"}`, "invalid JSON"} {
		if got := generatedSubjectKey(raw, nil); got != "" {
			t.Fatalf("无效响应被接受: %q", got)
		}
	}
}

func TestSubjectKeySuffix(t *testing.T) {
	seen := map[string]bool{}
	for _, base := range []string{"a", strings.Repeat("a", 64)} {
		for i := 0; i < 100; i++ {
			key := suffixedSubjectKey(base)
			if !namespacePattern.MatchString(key) || seen[key] || key == base {
				t.Fatal(key)
			}
			seen[key] = true
		}
	}
}

func TestJobErrorSummary(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"context deadline exceeded", "任务执行超时"},
		{"context canceled", "任务执行取消"},
		{"请求百炼模型: connection refused", "模型服务请求失败"},
		{"抽取 JSON 无效", "模型响应格式无效"},
		{"no rows in result set", "任务关联数据不存在"},
		{"", "任务执行失败"},
		{"未知错误", "任务执行失败"},
	} {
		// 恶意错误正文包含各种敏感字段，返回值必须仍然只是固定类别。
		raw := tc.raw + ` request_body={"content":"私人记忆","message":"私人消息","api_key":"secret-marker"}` + strings.Repeat("长", 300)
		if got := jobErrorSummary(raw); got != tc.want || len([]rune(got)) > 200 {
			t.Fatal(got)
		}
	}
}
