package config

import (
	"fmt"
	"github.com/caarlos0/env/v11"
	"time"
)

// KnowledgeConfig 集中定义摄取配额、切片参数和后台任务运行边界。
type KnowledgeConfig struct {
	MaxFileBytes       int64         `env:"MAX_FILE_BYTES" envDefault:"20971520"`
	MaxPDFPages        int           `env:"MAX_PDF_PAGES" envDefault:"300"`
	MaxCharacters      int           `env:"MAX_CHARACTERS" envDefault:"500000"`
	MaxDocuments       int           `env:"MAX_DOCUMENTS" envDefault:"100"`
	MaxBasesPerUser    int           `env:"MAX_BASES_PER_USER" envDefault:"10"`
	MaxUserBytes       int64         `env:"MAX_USER_BYTES" envDefault:"209715200"`
	MinPDFCharacters   int           `env:"MIN_PDF_CHARACTERS" envDefault:"50"`
	TargetTokens       int           `env:"TARGET_TOKENS" envDefault:"500"`
	MinTokens          int           `env:"MIN_TOKENS" envDefault:"50"`
	OverlapPercent     int           `env:"OVERLAP_PERCENT" envDefault:"15"`
	EmbeddingBatchSize int           `env:"EMBEDDING_BATCH_SIZE" envDefault:"20"`
	TextType           string        `env:"TEXT_TYPE" envDefault:"document"`
	JobTimeout         time.Duration `env:"JOB_TIMEOUT" envDefault:"10m"`
	OrphanGrace        time.Duration `env:"ORPHAN_GRACE" envDefault:"24h"`
	CleanupInterval    time.Duration `env:"CLEANUP_INTERVAL" envDefault:"1h"`
	TempDir            string        `env:"TEMP_DIR" envDefault:"../.output/kb"`
}

// DefaultKnowledge 返回与环境加载一致的默认值，供离线接口导出和纯函数测试使用。
func DefaultKnowledge() KnowledgeConfig {
	c, err := env.ParseAsWithOptions[KnowledgeConfig](env.Options{Environment: map[string]string{}})
	if err != nil {
		panic(err)
	}
	return c
}

// Validate 拒绝无法执行的配额和切片配置。
func (c KnowledgeConfig) Validate() error {
	if c.MaxFileBytes <= 0 || c.MaxPDFPages <= 0 || c.MaxCharacters <= 0 || c.MaxDocuments <= 0 || c.MaxBasesPerUser <= 0 || c.MaxUserBytes <= 0 || c.MinPDFCharacters <= 0 {
		return fmt.Errorf("知识库配额必须大于零")
	}
	if c.MinTokens <= 0 || c.TargetTokens < 2*c.MinTokens || c.OverlapPercent < 0 || c.OverlapPercent >= 50 {
		return fmt.Errorf("知识库切片参数无效")
	}
	if c.EmbeddingBatchSize < 1 || c.EmbeddingBatchSize > 20 {
		return fmt.Errorf("知识库向量批次必须在 1–20 之间")
	}
	if c.TextType != "" && c.TextType != "document" {
		return fmt.Errorf("知识库 text_type 只能为 document 或空")
	}
	if c.JobTimeout <= 0 || c.OrphanGrace <= c.JobTimeout || c.CleanupInterval <= 0 || c.TempDir == "" {
		return fmt.Errorf("知识库任务超时、清理间隔或临时目录无效")
	}
	return nil
}
