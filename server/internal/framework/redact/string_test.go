package redact

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	// 随机测试串不是实际凭证，验证显式秘密以及协议标记都不会保留。
	secret := rand.Text()
	input := "prefix " + secret + " sk-" + secret + " Bearer " + secret + " suffix"
	got := String(input, secret)
	if strings.Contains(got, secret) || strings.Contains(got, "sk-") || strings.Contains(got, "Bearer") {
		t.Fatal("脱敏结果仍含敏感内容")
	}
	if String("普通错误") != "普通错误" {
		t.Fatal("不应改变普通描述")
	}
}
