package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// Embed 使用百炼固定的 1024 维向量，错误响应不回显供应商正文。
func (m *BailianModel) Embed(ctx context.Context, input string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{"model": "qwen3.7-text-embedding", "input": input, "dimensions": 1024, "encoding_format": "float"})
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
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) != 1 || len(result.Data[0].Embedding) != 1024 {
		return nil, fmt.Errorf("向量维度必须为 1024")
	}
	var norm float64
	for _, v := range result.Data[0].Embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("向量包含非有限值")
		}
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return nil, fmt.Errorf("向量不能为零")
	}
	return result.Data[0].Embedding, nil
}
