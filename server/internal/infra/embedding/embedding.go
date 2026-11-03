package embedding

import "context"

// Client 提供文本向量生成能力。
type Client interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
