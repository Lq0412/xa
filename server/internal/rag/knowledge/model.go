package knowledge

import "time"

// KnowledgeBase 表示一个可绑定到 Agent 的知识库。
type KnowledgeBase struct {
	ID          string
	WorkspaceID string
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Document 表示知识库中的原始文档。
type Document struct {
	ID              string
	KnowledgeBaseID string
	ObjectKey       string
	MediaType       string
	Checksum        string
	Status          string
}
