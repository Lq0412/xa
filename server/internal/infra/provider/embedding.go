package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// EmbeddingModel 固定知识库与记忆共用的向量模型及维度约定。
const EmbeddingModel = "qwen3.7-text-embedding"

// Embed 保留记忆的单条接口，不改变其 text_type 行为。
func (m *BailianModel) Embed(ctx context.Context, input string) ([]float32, error) {
	vectors, err := m.EmbedBatch(ctx, []string{input}, "")
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

// EmbedBatch 一次提交最多 20 条文本，按供应商 index 恢复输入顺序，不记录输入或响应正文。
func (m *BailianModel) EmbedBatch(ctx context.Context, inputs []string, textType string) ([][]float32, error) {
	if len(inputs) < 1 || len(inputs) > 20 {
		return nil, fmt.Errorf("向量批次必须包含 1–20 条文本")
	}
	if textType != "" && textType != "document" && textType != "query" {
		return nil, fmt.Errorf("text_type 必须为 document 或 query")
	}
	payload := map[string]any{"model": EmbeddingModel, "input": inputs, "dimensions": 1024, "encoding_format": "float"}
	if textType != "" {
		payload["text_type"] = textType
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.BaseURL+"/embeddings", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求向量模型失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("向量模型 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) != len(inputs) {
		return nil, fmt.Errorf("向量批次返回数量不匹配")
	}
	vectors := make([][]float32, len(inputs))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(inputs) || vectors[item.Index] != nil || len(item.Embedding) != 1024 {
			return nil, fmt.Errorf("向量索引或 1024 维度无效")
		}
		var norm float64
		for _, v := range item.Embedding {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, fmt.Errorf("向量包含非有限值")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return nil, fmt.Errorf("向量不能为零")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}
