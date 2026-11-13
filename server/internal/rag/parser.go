// Package rag 管理独立知识库及文档摄取，不依赖 Agent 运行或记忆存储。
package rag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"linq-bs/server/internal/framework/config"
)

// Fault 将用户可读 Message 与后台排查使用的 cause 分开。
type Fault struct {
	Code    string
	Message string
	Status  int
	cause   error
}

func (e *Fault) Error() string {
	if e.cause != nil {
		return e.Message + ": " + e.cause.Error()
	}
	return e.Message
}

// Unwrap 保留内部失败原因供后台日志排查；接口和文档状态只使用 Message。
func (e *Fault) Unwrap() error { return e.cause }
func fault(code, message string, status int) error {
	return &Fault{Code: code, Message: message, Status: status}
}

// Section 标记规范化文本中的连续区域；字符位置以 Unicode 字符计数、左闭右开。
type Section struct {
	Content     string
	HeadingPath []string
	Page        *int
	CharStart   int
}

// Parsed 保存解析后正文和出处；PDF 的页分隔符占一个字符。
type Parsed struct {
	Sections  []Section
	PageCount int
}

// ValidateFile 在上传边界同时校验扩展名、内容嗅探和 UTF-8；不信任客户端 MIME。
func ValidateFile(filename string, data []byte, cfg config.KnowledgeConfig) (string, error) {
	if int64(len(data)) > cfg.MaxFileBytes {
		return "", fault("file_too_large", "文件超过知识库单文件大小限制", 413)
	}
	if len(data) == 0 {
		return "", fault("empty_document", "文件没有内容", 400)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	detected := http.DetectContentType(data)
	switch ext {
	case ".pdf":
		if detected != "application/pdf" {
			return "", fault("invalid_file_type", "PDF 扩展名与文件内容不符", 400)
		}
		return "application/pdf", nil
	case ".txt", ".md":
		if !utf8.Valid(data) {
			return "", fault("invalid_utf8", "文本必须是合法 UTF-8 编码", 400)
		}
		// HTML/XML 等可执行或结构化文档不冒充纯文本；Markdown 仍按普通文本保存。
		if !strings.HasPrefix(detected, "text/plain") {
			return "", fault("invalid_file_type", "文件内容不是支持的纯文本或 Markdown", 400)
		}
		if ext == ".md" {
			return "text/markdown", nil
		}
		return "text/plain", nil
	default:
		return "", fault("unsupported_file_type", "仅支持 .txt、.md 和有文本层的 .pdf 文件", 400)
	}
}

// Parse 使用 Poppler 抽取 PDF 文本，使用 Goldmark 识别 Markdown 标题结构。
func Parse(ctx context.Context, data []byte, mime string, cfg config.KnowledgeConfig) (Parsed, error) {
	var pages []string
	result := Parsed{}
	if mime == "application/pdf" {
		if err := os.MkdirAll(cfg.TempDir, 0700); err != nil {
			return result, err
		}
		file, err := os.CreateTemp(cfg.TempDir, "document-*.pdf")
		if err != nil {
			return result, err
		}
		defer os.Remove(file.Name())
		if _, err = file.Write(data); err != nil {
			file.Close()
			return result, err
		}
		if err = file.Close(); err != nil {
			return result, err
		}
		output := &boundedOutput{limit: 4 * (cfg.MaxCharacters + cfg.MaxPDFPages)}
		cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", file.Name(), "-")
		cmd.Stdout = output
		cmd.Stderr = io.Discard
		if err = cmd.Run(); err != nil {
			if output.exceeded {
				return result, fault("too_many_characters", "解析后的文字超过字数限制", 400)
			}
			return result, fault("pdf_parse_failed", "PDF 无法解析，请确认文件完整且没有密码保护", 400)
		}
		pages = strings.Split(output.String(), "\f")
		if pages[len(pages)-1] == "" {
			pages = pages[:len(pages)-1]
		}
		result.PageCount = len(pages)
		if len(pages) > cfg.MaxPDFPages {
			return result, fault("too_many_pages", "PDF 超过页数限制", 400)
		}
		visible := 0
		for _, page := range pages {
			for _, r := range page {
				if !unicode.IsSpace(r) {
					visible++
				}
			}
		}
		if len(pages) == 0 || visible < len(pages)*cfg.MinPDFCharacters {
			return result, fault("scanned_pdf", "扫描件，暂不支持 OCR", 400)
		}
	} else {
		pages = []string{strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\ufeff")}
	}
	offset := 0
	for i, content := range pages {
		if !utf8.ValidString(content) {
			return result, fault("invalid_utf8", "解析文本不是合法 UTF-8 编码", 400)
		}
		count := utf8.RuneCountInString(content)
		if offset+count > cfg.MaxCharacters {
			return result, fault("too_many_characters", "解析后的文字超过字数限制", 400)
		}
		if mime == "text/markdown" {
			result.Sections = markdownSections(content)
		} else {
			var page *int
			if mime == "application/pdf" {
				n := i + 1
				page = &n
			}
			result.Sections = append(result.Sections, Section{Content: content, HeadingPath: []string{}, Page: page, CharStart: offset})
		}
		offset += count + 1
	}
	if strings.TrimSpace(strings.Join(pages, "")) == "" {
		return result, fault("empty_document", "文档没有可摄取的文字", 400)
	}
	return result, nil
}

// boundedOutput 限制外部解析器的输出内存；达到解析上限便停止复制。
type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedOutput) String() string { return b.buffer.String() }
func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > b.limit {
		b.exceeded = true
		return 0, fmt.Errorf("解析输出超过限制")
	}
	return b.buffer.Write(p)
}

func markdownSections(content string) []Section {
	raw := []byte(content)
	root := goldmark.DefaultParser().Parse(text.NewReader(raw))
	sections := []Section{}
	path := []string{}
	levels := []int{}
	start := 0
	appendSection := func(end int) {
		if end > start {
			sections = append(sections, Section{Content: string(raw[start:end]), HeadingPath: append([]string{}, path...), CharStart: utf8.RuneCount(raw[:start])})
		}
	}
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok {
			continue
		}
		// Markdown 允许只有 # 的空标题；它不改变引用路径，正文仍保留原字符位置。
		if heading.Lines().Len() == 0 {
			continue
		}
		pos := heading.Lines().At(0).Start
		pos = bytes.LastIndexByte(raw[:pos], '\n') + 1
		appendSection(pos)
		for len(levels) > 0 && levels[len(levels)-1] >= heading.Level {
			levels = levels[:len(levels)-1]
			path = path[:len(path)-1]
		}
		levels = append(levels, heading.Level)
		path = append(path, string(heading.Text(raw)))
		start = pos
	}
	appendSection(len(raw))
	return sections
}
