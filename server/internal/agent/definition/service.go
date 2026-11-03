package definition

import (
	"context"
	"linq-bs/server/internal/framework/auth"
)

// SaveInput 将资料与完整配置一起保存；创建时 AgentID 为空。
// ExpectedVersionID 检测并发修改，身份和归属由服务端确定。
type SaveInput struct {
	AgentID           string
	ExpectedVersionID string
	Name              string
	Avatar            string
	Description       string
	Tags              []string
	Visibility        Visibility
	Configuration     Configuration
}

// Saved 是成功保存后的定义和不可变版本。
type Saved struct {
	Definition AgentDefinition
	Version    AgentVersion
}

// Service 校验归属、配置与资源权限；保存直接生效，无单独发布流程。
// 停用和删除必须阻止新运行并取消已有运行；历史数据保持原归属。
type Service interface {
	Save(ctx context.Context, actor auth.Principal, input SaveInput) (Saved, error)
	Get(ctx context.Context, actor auth.Principal, agentID string) (AgentDefinition, error)
	List(ctx context.Context, actor auth.Principal) ([]AgentDefinition, error)
	Validate(ctx context.Context, actor auth.Principal, input SaveInput) error
	Enable(ctx context.Context, actor auth.Principal, agentID string) error
	Disable(ctx context.Context, actor auth.Principal, agentID string) error
	Delete(ctx context.Context, actor auth.Principal, agentID string) error
}
