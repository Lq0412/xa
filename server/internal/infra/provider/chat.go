package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Message 是供应商无关的对话消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage 记录模型返回的 token 用量。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ChatModel 以增量回调生成回复，回调返回错误时立即停止上游请求。
type ChatModel interface {
	Stream(ctx context.Context, model string, messages []Message, delta func(string) error) (Usage, error)
}

// BailianModel 调用阿里百炼 OpenAI 兼容 chat/completions 接口。
type BailianModel struct {
	APIKey, BaseURL string
	Client          *http.Client
}

func NewBailian(cfgAPIKey, baseURL string) *BailianModel {
	return &BailianModel{APIKey: cfgAPIKey, BaseURL: strings.TrimRight(baseURL, "/"), Client: http.DefaultClient}
}

func (m *BailianModel) Stream(ctx context.Context, model string, messages []Message, delta func(string) error) (Usage, error) {
	return m.stream(ctx, model, messages, nil, delta)
}

// StreamStructured 使用供应商严格 JSON Schema 模式，仍关闭思考。
func (m *BailianModel) StreamStructured(ctx context.Context, model string, messages []Message, schema json.RawMessage, delta func(string) error) (Usage, error) {
	return m.stream(ctx, model, messages, map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "memory_operations", "strict": true, "schema": schema}}, delta)
}

func (m *BailianModel) stream(ctx context.Context, model string, messages []Message, format any, delta func(string) error) (Usage, error) {
	body := struct {
		Model          string    `json:"model"`
		Messages       []Message `json:"messages"`
		Stream         bool      `json:"stream"`
		EnableThinking bool      `json:"enable_thinking"`
		ResponseFormat any       `json:"response_format,omitempty"`
	}{model, messages, true, false, format}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Usage{}, fmt.Errorf("编码模型请求: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.BaseURL+"/chat/completions", strings.NewReader(string(encoded)))
	if err != nil {
		return Usage{}, fmt.Errorf("创建模型请求: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.Client.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("请求百炼模型: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Usage{}, fmt.Errorf("百炼模型返回 %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var usage Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return usage, fmt.Errorf("解析模型流: %w", err)
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if err := delta(chunk.Choices[0].Delta.Content); err != nil {
				return usage, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, fmt.Errorf("读取模型流: %w", err)
	}
	return usage, nil
}
