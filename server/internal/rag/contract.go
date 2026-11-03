package rag

// DocumentReference 表示一次检索返回的知识片段。
type DocumentReference struct {
	DocumentID string
	ChunkID    string
	Content    string
	Score      float64
}

// Retriever 是 Agent 访问知识库的唯一领域接口。
type Retriever interface {
	Retrieve(query string, limit int) ([]DocumentReference, error)
}
