package redact

import (
	"regexp"
	"strings"
)

var credentials = regexp.MustCompile(`(?i)(sk-[^\s"'<>]+|Bearer\s+[^\s"'<>]+)`)

// 标签后的整段值保守隐藏，覆盖带空格的密码和 JSON 引号；不参与记忆写入判定。
var labeledSecret = regexp.MustCompile(`(?i)(api[_ -]?key|authorization|password|密码|银行卡|身份证)["']?\s*(?:[:=：]|是)?\s*[^\r\n,，;；}]+`)
var email = regexp.MustCompile(`[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+`)
var phone = regexp.MustCompile(`\b(?:\+?86[- ]?)?1[3-9][0-9]{9}\b`)
var identity = regexp.MustCompile(`\b(?:[0-9]{16,19}|[0-9]{17}[0-9xX])\b`)

// String 在截断之前移除显式秘密及常见凭证形式，避免凭证前缀或尾部残留。
func String(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	for _, pattern := range []*regexp.Regexp{labeledSecret, credentials, email, phone, identity} {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}
