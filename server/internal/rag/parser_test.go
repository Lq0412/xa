package rag

import (
	"context"
	"errors"
	"linq-bs/server/internal/framework/config"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestFileBoundary 覆盖扩展名与内容不符、非法 UTF-8 和上传字节上限。
func TestFileBoundary(t *testing.T) {
	cfg := config.DefaultKnowledge()
	for _, tc := range []struct {
		name string
		data []byte
		code string
	}{
		{"中文.txt", []byte("中文知识库。"), ""}, {"笔记.md", []byte("# 标题\n这是中文笔记。"), ""},
		{"伪装.pdf", []byte("并不是PDF"), "invalid_file_type"}, {"伪装.txt", []byte("%PDF-1.4\n"), "invalid_file_type"},
		{"编码.txt", []byte{0xff, 0xfe}, "invalid_utf8"}, {"表格.docx", []byte("文字"), "unsupported_file_type"},
	} {
		_, err := ValidateFile(tc.name, tc.data, cfg)
		if tc.code == "" {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		var f *Fault
		if !errors.As(err, &f) || f.Code != tc.code {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	cfg.MaxFileBytes = 3
	if _, err := ValidateFile("超限.txt", []byte("中文"), cfg); err == nil {
		t.Fatal("超限文件被接受")
	}
}

// TestMarkdownProvenance 验证标题层级、中文字符位置与有限重叠，不丢正文。
func TestMarkdownProvenance(t *testing.T) {
	cfg := config.DefaultKnowledge()
	content := "# 课程\n导语。\n\n## 口语\n" + strings.Repeat("练习时需要先回答问题，再补充具体例子。\n\n", 120) + "\n## 阅读\n" + strings.Repeat("阅读时先找文章主题。", 30)
	parsed, err := Parse(context.Background(), []byte(content), "text/markdown", cfg)
	if err != nil {
		t.Fatal(err)
	}
	chunks := Split(parsed, cfg)
	if len(chunks) < 4 {
		t.Fatal("长文没有切分")
	}
	runes := []rune(content)
	covered := make([]bool, len(runes))
	overlap := false
	heading := false
	for i, c := range chunks {
		if string(runes[c.CharStart:c.CharEnd]) != c.Content {
			t.Fatal("字符位置不能还原内容")
		}
		if c.TokenCount <= 0 || c.TokenCount > cfg.TargetTokens+cfg.MinTokens {
			t.Fatalf("切片大小失控: %d", c.TokenCount)
		}
		if strings.Join(c.HeadingPath, "/") == "课程/口语" {
			heading = true
		}
		if i > 0 && c.CharStart < chunks[i-1].CharEnd {
			overlap = true
		}
		for pos := c.CharStart; pos < c.CharEnd; pos++ {
			covered[pos] = true
		}
	}
	for i, r := range runes {
		if !covered[i] && !strings.ContainsRune(" \n\t\r", r) {
			t.Fatalf("正文第 %d 字丢失", i)
		}
	}
	if !heading || !overlap {
		t.Fatal("缺少标题路径或重叠")
	}
	if !utf8.ValidString(chunks[1].Content) {
		t.Fatal("切开了UTF-8字符")
	}
	cfg.MaxCharacters = 10
	if _, err = Parse(context.Background(), []byte(content), "text/markdown", cfg); err == nil {
		t.Fatal("解析字符数超限未失败")
	}
	for _, content := range []string{"#\n\n正文。", "标题\n====\n\n段落。", "```md\n# 代码里的标题\n```\n\n正文。"} {
		parsed, err := Parse(context.Background(), []byte(content), "text/markdown", config.DefaultKnowledge())
		if err != nil || len(Split(parsed, config.DefaultKnowledge())) == 0 {
			t.Fatalf("合法 Markdown 解析失败: %v", err)
		}
	}
}

// TestChunkBoundaries 过长句子仍受限制，页之间不互相重叠。
func TestChunkBoundaries(t *testing.T) {
	cfg := config.DefaultKnowledge()
	page1, page2 := 1, 2
	parsed := Parsed{Sections: []Section{{Content: strings.Repeat("中", 1800), Page: &page1}, {Content: strings.Repeat("文", 850), Page: &page2, CharStart: 1801}}}
	chunks := Split(parsed, cfg)
	for _, c := range chunks {
		if c.PageFrom == nil || *c.PageFrom != *c.PageTo {
			t.Fatal("页码不正确")
		}
		if c.TokenCount > cfg.TargetTokens+cfg.MinTokens {
			t.Fatal("长句未受大小限制")
		}
	}
	b := &boundedOutput{limit: 4}
	if _, err := b.Write([]byte("12345")); err == nil || !b.exceeded {
		t.Fatal("外部解析输出不受限制")
	}
}
