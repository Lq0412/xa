package provider

// Name 返回适配器标识，不包含模型名或凭证。
func (m *BailianModel) Name() string { return "bailian" }

// Capabilities 标记已实测协议能力；工具数及 token 数均为本地保守预算【未验证厂商上限】。
func (m *BailianModel) Capabilities(model string) Capabilities {
	limits, ok := m.Limits[model]
	if !ok {
		limits = TokenLimits{ContextTokens: 32768, MaxInputTokens: 28672, OutputReserveTokens: 4096}
	}
	return Capabilities{Tools: true, ParallelToolCalls: true, ToolChoiceNone: true, ToolChoiceFunction: true,
		ReliableFunctionChoice: false, ToolChoiceRequired: false, MaxTools: 32,
		ContextTokens: limits.ContextTokens, MaxInputTokens: limits.MaxInputTokens, OutputReserveTokens: limits.OutputReserveTokens}
}
