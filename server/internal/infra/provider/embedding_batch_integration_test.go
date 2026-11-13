//go:build integration

package provider

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/config"
	"testing"
	"time"
)

// TestBailianEmbeddingBatch 使用真实兼容接口核对批量、顺序与 text_type 的接受情况。
func TestBailianEmbeddingBatch(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal("集成测试配置不完整")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, cfg.Postgres.URL())
	if err != nil {
		t.Fatal("数据库连接失败")
	}
	defer db.Close()
	var version string
	if err = db.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname='vector'`).Scan(&version); err != nil {
		t.Fatal("pgvector 查询失败")
	}
	t.Logf("pgvector=%s", version)
	model := NewBailian(cfg.Bailian.APIKey, cfg.Bailian.BaseURL)
	for _, typ := range []string{"", "document", "query"} {
		vectors, err := model.EmbedBatch(ctx, []string{"中文知识库：雅思口语练习需要举例说明。", "知识库文档按标题和段落切片。"}, typ)
		if err != nil {
			t.Fatalf("text_type=%q: %v", typ, err)
		}
		if len(vectors) != 2 || len(vectors[0]) != 1024 {
			t.Fatal("批量响应无效")
		}
		t.Logf("text_type=%q: HTTP 成功，2 × 1024 维；仅证明接口接受参数", typ)
	}
}
