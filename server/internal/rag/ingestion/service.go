package ingestion

import "context"

// Pipeline 负责文档解析、分块、向量生成和索引写入。
type Pipeline interface {
	Ingest(ctx context.Context, documentID string) error
}
