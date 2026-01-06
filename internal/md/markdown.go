package md

import (
	"bytes"
	"log"
	"strings"

	mathjax "github.com/litao91/goldmark-mathjax" // 识别数学公式
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-meta" // 解析 Frontmatter
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/toc"
)

type MarkdownResult struct {
	Meta     map[string]interface{}
	Markdown string
	HTML     string
}

func GetMarkdownOnly(input string) string {
	if !strings.HasPrefix(input, "---") {
		return input
	}

	// 找到第二个 "---" 的位置
	parts := strings.SplitN(input, "---", 3)
	if len(parts) < 3 {
		return input
	}
	return strings.TrimSpace(parts[2])
}

func ParseMarkdown(text string) *MarkdownResult {
	md := goldmark.New(
		goldmark.WithExtensions(
			meta.Meta,       // 开启 Frontmatter 支持
			mathjax.MathJax, // 开启公式支持，它会把 $$ 内部内容原样保留输出
			&toc.Extender{},
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // 关键：允许渲染原始 HTML 和不安全的标签
		),
	)

	source := []byte(text)

	var buf bytes.Buffer
	context := parser.NewContext()
	if err := md.Convert(source, &buf, parser.WithContext(context)); err != nil {
		log.Fatalf("md.Convert: %s", err)
	}

	metaData := meta.Get(context)
	//for s := range metaData {
	//	fmt.Printf("%s %s %T\n", s, metaData[s], metaData[s])
	//}
	//
	//// 4. 获取 HTML 内容
	//fmt.Println("--- HTML 内容 ---")
	//fmt.Println(buf.String())
	return &MarkdownResult{
		Meta:     metaData,
		Markdown: GetMarkdownOnly(text),
		HTML:     buf.String(),
	}
}
