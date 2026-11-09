package provider

import (
	"context"
	"strings"
)

// Router 按最长模型名前缀选择适配器；无匹配时仅使用显式配置的 Default。
// 构造完成后不再修改 ByPrefix，允许多会话并发读取。
type Router struct {
	ByPrefix map[string]ChatProvider
	Default  ChatProvider
}

func (r *Router) selectProvider(model string) ChatProvider {
	selected, length := r.Default, -1
	for prefix, candidate := range r.ByPrefix {
		if strings.HasPrefix(model, prefix) && len(prefix) > length {
			selected, length = candidate, len(prefix)
		}
	}
	return selected
}

// Name 返回路由器标识，单次调用的供应商由模型名前缀决定。
func (r *Router) Name() string { return "router" }

// Capabilities 返回被选中适配器的能力；没有注册的模型不声明任何能力。
func (r *Router) Capabilities(model string) Capabilities {
	if p := r.selectProvider(model); p != nil {
		return p.Capabilities(model)
	}
	return Capabilities{}
}

// Chat 将完整请求与事件接收方交给同一个选定的适配器。
func (r *Router) Chat(ctx context.Context, req ChatRequest, sink func(StreamEvent) error) (Response, error) {
	p := r.selectProvider(req.Model)
	if p == nil {
		return Response{}, &ProviderError{Kind: ProviderErrorBadRequest, Message: "模型未注册供应商"}
	}
	return p.Chat(ctx, req, sink)
}

var _ ChatProvider = (*Router)(nil)
var _ ChatProvider = (*BailianModel)(nil)
