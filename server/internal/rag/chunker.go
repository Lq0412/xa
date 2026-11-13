package rag

import (
	"fmt"
	"linq-bs/server/internal/framework/config"
	"strings"
	"unicode"
)

// Chunk 保存切片正文及未来引用需要的稳定出处；不向 API 返回向量。
type Chunk struct {
	ID              string    `json:"id"`
	DocumentID      string    `json:"document_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	Index           int       `json:"chunk_index"`
	Content         string    `json:"content"`
	SearchText      string    `json:"-"`
	HeadingPath     []string  `json:"heading_path"`
	PageFrom        *int      `json:"page_from"`
	PageTo          *int      `json:"page_to"`
	CharStart       int       `json:"char_start" doc:"规范化文本中的 Unicode 字符起点，包含此位置"`
	CharEnd         int       `json:"char_end" doc:"规范化文本中的 Unicode 字符终点，不包含此位置"`
	TokenCount      int       `json:"token_count" doc:"切片估算 token 数"`
	EmbedModel      string    `json:"embed_model"`
	Embedding       []float32 `json:"-"`
}

// ChunkerVersion 固定算法、估算器、分词字典与参数；版本变化只预留 rebuild，不自动执行。
func ChunkerVersion(c config.KnowledgeConfig) string {
	return fmt.Sprintf("recursive-v1-cjk2of3-gse0.80.3-zh-t%d-o%d-m%d", c.TargetTokens, c.OverlapPercent, c.MinTokens)
}

// tokenUnits 用十二分之一 token 计量：中文约 1.5 字/token，ASCII 约 4 字/token。
func tokenUnits(r rune) int {
	if r <= unicode.MaxASCII {
		return 3
	}
	return 8
}
func tokenCount(runes []rune) int {
	n := 0
	for _, r := range runes {
		n += tokenUnits(r)
	}
	return (n + 11) / 12
}

// Split 按标题或页分组后递归寻找段落、句子、字符边界；重叠只发生在同一组内。
func Split(parsed Parsed, cfg config.KnowledgeConfig) []Chunk {
	chunks := []Chunk{}
	for _, section := range parsed.Sections {
		runes := []rune(section.Content)
		for start := 0; start < len(runes); {
			for start < len(runes) && unicode.IsSpace(runes[start]) {
				start++
			}
			if start == len(runes) {
				break
			}
			end := start
			units := 0
			for end < len(runes) && units+tokenUnits(runes[end]) <= cfg.TargetTokens*12 {
				units += tokenUnits(runes[end])
				end++
			}
			if end < len(runes) {
				end = splitBoundary(runes, start, end, cfg.MinTokens)
			}
			// 尾部不足最小大小时并入本块，最多增加 min_tokens，避免零碎尾块。
			if end < len(runes) && tokenCount(runes[end:]) < cfg.MinTokens {
				end = len(runes)
			}
			trimmed := end
			for trimmed > start && unicode.IsSpace(runes[trimmed-1]) {
				trimmed--
			}
			chunks = append(chunks, Chunk{Index: len(chunks), Content: string(runes[start:trimmed]), HeadingPath: append([]string{}, section.HeadingPath...), PageFrom: section.Page, PageTo: section.Page, CharStart: section.CharStart + start, CharEnd: section.CharStart + trimmed, TokenCount: tokenCount(runes[start:trimmed])})
			if end == len(runes) {
				break
			}
			next := end
			overlap := tokenCount(runes[start:end]) * cfg.OverlapPercent / 100 * 12
			for next > start+1 && overlap >= tokenUnits(runes[next-1]) {
				next--
				overlap -= tokenUnits(runes[next])
			}
			start = next
		}
	}
	return chunks
}

func splitBoundary(runes []rune, start, end, minTokens int) int {
	lower := start
	units := 0
	for lower < end && units < minTokens*12 {
		units += tokenUnits(runes[lower])
		lower++
	}
	for level := 0; level < 2; level++ {
		for i := end; i > lower; i-- {
			if level == 0 && runes[i-1] == '\n' && i > start+1 && runes[i-2] == '\n' {
				return i
			}
			if level == 1 && strings.ContainsRune("。！？；\n", runes[i-1]) {
				return i
			}
		}
	}
	return end
}
