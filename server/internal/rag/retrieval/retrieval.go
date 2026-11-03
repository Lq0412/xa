package retrieval

import (
	"context"

	"linq-bs/server/internal/rag"
)

// Service 实现知识检索和检索记录。
type Service interface {
	Retrieve(ctx context.Context, knowledgeBaseID string, query string, limit int) ([]rag.DocumentReference, error)
}
