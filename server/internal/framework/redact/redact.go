package redact

// Text 与 String 共用脱敏规则，供错误和追踪预览使用。
func Text(value string) string { return String(value) }
