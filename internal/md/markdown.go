package md

import (
	"bytes"
	"log"
	"strings"

	highlighting "github.com/yuin/goldmark-highlighting/v2"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-meta" // 解析 Frontmatter
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
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

func ParseMarkdown(text string, includeTOC bool) *MarkdownResult {
	if !includeTOC {
		includeTOC = true
	}
	extensions := []goldmark.Extender{
		meta.Meta, // 开启 Front matter 支持
		//mathjax.MathJax, // 开启公式支持，它会把 $$ 内部内容原样保留输出
		extension.Table,
		extension.CJK,
		extension.GFM,
		extension.Footnote,
		extension.Typographer,
		extension.Strikethrough,
		highlighting.NewHighlighting(
			highlighting.WithStyle("trac"),
			//highlighting.WithFormatOptions(
			//	chromahtml.WithLineNumbers(true),
			//),
		),
	}

	if includeTOC {
		//extensions = append(extensions, &toc.Extender{
		//	//Title:   "TOC-TITLE",
		//	TitleID: "toc-title",
		//	ListID:  "toc-list",
		//})
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			//parser.WithIDs(NewUnicodeIDs()),
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&MyTransformer{}, 100),
			),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // 关键：允许渲染原始 HTML 和不安全的标签
			renderer.WithNodeRenderers(
				util.Prioritized(&TOCContainerHTMLRenderer{}, 100),
			),
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	source := []byte(text)

	var buf bytes.Buffer
	context := parser.NewContext(parser.WithIDs(NewUnicodeIDs()))
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
