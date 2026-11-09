package definition

// ChatEnableThinking 对话是否开启百炼深度思考；未配置时默认开启。
func ChatEnableThinking(cfg Configuration) bool {
	if cfg.EnableThinking == nil {
		return true
	}
	return *cfg.EnableThinking
}
