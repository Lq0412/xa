// Package storage 提供 S3 对象访问，不在业务进程中隐式创建存储桶。
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"linq-bs/server/internal/framework/config"
)

// ObjectStore 保存音频和知识库原始文件。
type ObjectStore interface {
	Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Client 包装 S3 SDK 并持有独立 HTTP 连接池。
type Client struct {
	client    *minio.Client
	bucket    string
	transport *http.Transport
}

// Open 创建客户端并验证存储桶已存在且凭证可用。
func Open(ctx context.Context, cfg config.ObjectStoreConfig) (*Client, error) {
	s, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.Ping(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func newClient(cfg config.ObjectStoreConfig) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure, Region: cfg.Region, Transport: transport,
	})
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("创建 S3 客户端: %w", err)
	}
	return &Client{client: client, bucket: cfg.Bucket, transport: transport}, nil
}

// InitializeBucket 仅供显式部署初始化使用；server 和 worker 不调用此函数。
func InitializeBucket(ctx context.Context, cfg config.ObjectStoreConfig) error {
	s, err := newClient(cfg)
	if err != nil {
		return err
	}
	defer s.Close()
	exists, err := s.client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("检查 S3 存储桶: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
		return fmt.Errorf("创建 S3 存储桶: %w", err)
	}
	return nil
}

// Ping 校验实际桶访问权限，不以端口连通代替可用性检查。
func (s *Client) Ping(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("访问 S3 存储桶: %w", err)
	}
	if !exists {
		return fmt.Errorf("S3 存储桶不存在，请先初始化")
	}
	return nil
}

// Put 上传已知长度的内容，上传失败直接返回 SDK 错误。
func (s *Client) Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, content, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Open 直接发起一次 GET，缺失对象立即返回错误；调用方必须关闭读取器。
func (s *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	// 此接口只需顺序读取，用 SDK 的直接流避免 HEAD 与 GET 之间的状态变化。
	object, _, _, err := (minio.Core{Client: s.client}).GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	return object, err
}

// Delete 删除指定对象，保留 S3 的幂等删除语义。
func (s *Client) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// Close 释放空闲 HTTP 连接，须在正在进行的对象操作结束后调用。
func (s *Client) Close() { s.transport.CloseIdleConnections() }
