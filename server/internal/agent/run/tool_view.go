package run

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/tool"
)

// ToolResultPayload 是回喂给模型的统一 JSON 结构。
type ToolResultPayload struct {
	OK         bool             `json:"ok"`
	Data       any              `json:"data,omitempty"`
	Truncated  bool             `json:"truncated,omitempty"`
	ResultRef  string           `json:"result_ref,omitempty"`
	TotalChars int              `json:"total_chars,omitempty"`
	Hint       string           `json:"hint,omitempty"`
	Error      *ToolResultError `json:"error,omitempty"`
	Omitted    bool             `json:"omitted,omitempty"`
	Summary    string           `json:"summary,omitempty"`
}

type ToolResultError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

// FormatRejectResult 生成校验拒绝的 tool 消息内容。
func FormatRejectResult(code, message string) string {
	p := ToolResultPayload{
		OK: false,
		Error: &ToolResultError{
			Code: code, Message: message, Retryable: true,
		},
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// FormatToolError 将 *tool.Error 转为 tool 消息 JSON。
func FormatToolError(err *tool.Error) string {
	p := ToolResultPayload{
		OK: false,
		Error: &ToolResultError{
			Code: err.Code, Message: err.Message, Retryable: err.Retryable,
		},
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// FormatSuccess 将工具成功结果格式化为模型视图，必要时截断并分配 result_ref。
func FormatSuccess(res tool.Result, entry tool.Entry, limits int, assignRef func() string) (modelContent string, dataJSON json.RawMessage, truncated bool, totalChars int, resultRef string) {
	raw, err := json.Marshal(res.Data)
	if err != nil {
		raw = []byte(`{}`)
	}
	dataJSON = raw
	totalChars = len([]rune(string(raw)))
	view := string(raw)
	if s, ok := res.Data.(string); ok {
		view = s
		totalChars = len([]rune(view))
	}
	if summarizer, ok := findSummarizer(entry.Spec.Name); ok {
		if v, ok2 := summarizer.ModelView(res, limits); ok2 {
			view = v
		}
	}
	tokBudget := limits
	if entry.Spec.ResultMaxTokens > 0 && entry.Spec.ResultMaxTokens < tokBudget {
		tokBudget = entry.Spec.ResultMaxTokens
	}
	if tokBudget <= 0 {
		tokBudget = 1500
	}
	if runtime.EstimateTokens(view) <= tokBudget {
		p := ToolResultPayload{OK: true, Data: json.RawMessage(view), Truncated: false}
		if !json.Valid([]byte(view)) {
			var data any
			_ = json.Unmarshal(raw, &data)
			p.Data = data
		} else {
			var data any
			_ = json.Unmarshal([]byte(view), &data)
			p.Data = data
		}
		b, _ := json.Marshal(p)
		return string(b), raw, false, totalChars, ""
	}
	truncated = true
	resultRef = assignRef()
	short := runtime.TruncateTokens(view, tokBudget)
	p := ToolResultPayload{
		OK: true, Data: short, Truncated: true,
		ResultRef: resultRef, TotalChars: totalChars,
		Hint: "如需更多内容，调用 tool_result_read",
	}
	b, _ := json.Marshal(p)
	return string(b), raw, true, totalChars, resultRef
}

func findSummarizer(name string) (tool.Summarizer, bool) {
	return nil, false
}

// NewResultRef 生成 tr_ 前缀的结果引用 ID。
func NewResultRef() string {
	return "tr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// HistoryOmitToolContent 将过长 tool 内容压缩为 omitted 形式。
func HistoryOmitToolContent(content string, budget int, resultRef string) string {
	if runtime.EstimateTokens(content) <= budget {
		return content
	}
	summary := runtime.TruncateTokens(content, budget)
	p := ToolResultPayload{OK: true, Omitted: true, ResultRef: resultRef, Summary: summary}
	b, _ := json.Marshal(p)
	return string(b)
}

// StoreMaxBytes 控制 tool_results.data 存储上限。
func StoreMaxBytes() int {
	if v := os.Getenv("XA_TOOL_RESULT_STORE_MAX_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1 << 20
}

// TruncateStoredData 在写入 PG 前截断过大的 data。
func TruncateStoredData(data []byte) ([]byte, bool) {
	max := StoreMaxBytes()
	if len(data) <= max {
		return data, false
	}
	return data[:max], true
}

// FormatInternalError 生成 internal_error 回喂内容。
func FormatInternalError(detail string) string {
	msg := "工具内部错误"
	if detail != "" {
		msg = fmt.Sprintf("工具内部错误: %s", detail)
	}
	return FormatRejectResult("internal_error", msg)
}
