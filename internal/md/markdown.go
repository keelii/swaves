package md

import (
	"bytes"
	"log"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark-meta" // 解析 Frontmatter
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// 自定义主题，基于 trac，但去掉背景色
var MyTracNoBg = styles.Register(chroma.MustNewStyle("mytrac",
	chroma.StyleEntries{
		chroma.Error:              "#a61717",
		chroma.Background:         "", // 不设置背景
		chroma.Keyword:            "bold",
		chroma.KeywordType:        "#445588",
		chroma.NameAttribute:      "#008080",
		chroma.NameBuiltin:        "#999999",
		chroma.NameClass:          "bold #445588",
		chroma.NameConstant:       "#008080",
		chroma.NameEntity:         "#800080",
		chroma.NameException:      "bold #990000",
		chroma.NameFunction:       "bold #990000",
		chroma.NameNamespace:      "#555555",
		chroma.NameTag:            "#000080",
		chroma.NameVariable:       "#008080",
		chroma.LiteralString:      "#bb8844",
		chroma.LiteralStringRegex: "#808000",
		chroma.LiteralNumber:      "#009999",
		chroma.Operator:           "bold",
		chroma.Comment:            "italic #999988",
		chroma.CommentSpecial:     "bold #999999",
		chroma.CommentPreproc:     "bold noitalic #999999",
		chroma.GenericDeleted:     "#000000",
		chroma.GenericEmph:        "italic",
		chroma.GenericError:       "#aa0000",
		chroma.GenericHeading:     "#999999",
		chroma.GenericInserted:    "#000000",
		chroma.GenericOutput:      "#888888",
		chroma.GenericPrompt:      "#555555",
		chroma.GenericStrong:      "bold",
		chroma.GenericSubheading:  "#aaaaaa",
		chroma.GenericTraceback:   "#aa0000",
		chroma.GenericUnderline:   "underline",
		chroma.TextWhitespace:     "#bbbbbb",
	},
))

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
			highlighting.WithCustomStyle(MyTracNoBg),
			//highlighting.WithWrapperRenderer(func(w util.BufWriter, ctx highlighting.CodeBlockContext, entering bool) {
			//	if entering {
			//		w.WriteString("<pre class=\"my-code\">") // 自定义外层
			//	} else {
			//		w.WriteString("</pre>")
			//	}
			//}),
			//highlighting.WithFormatOptions(
			//	chromahtml.WithLineNumbers(true),
			//),
		),
	}

	options := []goldmark.Option{
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			//parser.WithIDs(NewUnicodeIDs()),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // 关键：允许渲染原始 HTML 和不安全的标签
			renderer.WithNodeRenderers(
				util.Prioritized(&TOCContainerHTMLRenderer{}, 100),
			),
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	}

	if includeTOC {
		options = append(options, goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&MyTransformer{}, 100),
			),
		))
	}

	md := goldmark.New(
		options...,
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
