package storage

import (
	"context"
	"io"
)

// ObjectStore 保存音频和知识库原始文件。
type ObjectStore interface {
	Put(ctx context.Context, key string, content io.Reader, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
