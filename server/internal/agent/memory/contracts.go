package memory

import (
	"context"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
)

// SourceType 保留写入来源，外部来源不得直接写入 user 共享层。
type SourceType string

const (
	SourceChat       SourceType = "chat_extraction"
	SourceUser       SourceType = "user_edit"
	SourceTool       SourceType = "tool"
	SourceImport     SourceType = "import"
	SourceDelegation SourceType = "delegation"
)

// Store 是已授权事实的读写边界；未来外部写入也必须由 Memory Service 校验。
type Store interface {
	List(context.Context, auth.Principal, Filter) ([]Item, error)
	Add(context.Context, auth.Principal, AddInput) (Item, error)
	Delete(context.Context, auth.Principal, string) error
}

// Extractor、Summarizer 隔离轻量模型能力；Retriever 隔离检索实现。
type Extractor interface {
	Extract(context.Context, []provider.Message, []Item) ([]Operation, error)
}
type Operation struct {
	Op       string
	Item     Item
	TargetID string
}
type Summarizer interface {
	Summarize(context.Context, string, []provider.Message, int) (string, error)
}
type Retriever interface {
	Retrieve(context.Context, auth.Principal, string, string, int) ([]Item, error)
}

// ShareGrant 仅描述后续授权表的接法，不启用跨 Agent 私有记忆读取。
type ShareGrant struct{ OwnerUserID, AgentID, SourceAgentID, Scope, Namespace string }
type ShareAuthorizer interface {
	ReadGrants(context.Context, auth.Principal, string) ([]ShareGrant, error)
}
