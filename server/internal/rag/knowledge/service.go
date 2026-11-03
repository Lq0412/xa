package knowledge

import "context"

// Repository 隔离知识库和文档持久化。
type Repository interface {
	Find(ctx context.Context, knowledgeBaseID string) (KnowledgeBase, error)
	Save(ctx context.Context, knowledgeBase KnowledgeBase) error
	SaveDocument(ctx context.Context, document Document) error
}

// Service 提供知识库管理用例。
type Service interface {
	Create(ctx context.Context, knowledgeBase KnowledgeBase) (KnowledgeBase, error)
	AddDocument(ctx context.Context, document Document) error
}
