package md

import (
	"bytes"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/helper"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
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
	TOCHTML  string
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
	//text = annotateFenceLanguages(text)

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
			highlighting.WithCustomStyle(SwavesTrac),
			highlighting.WithGuessLanguage(true),
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
			//html.WithHardWraps(),
			html.WithXHTML(),
			renderer.WithNodeRenderers(
				util.Prioritized(&TOCContainerHTMLRenderer{}, 100),
				util.Prioritized(&FigureRenderer{}, 500),
			),
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
		logger.Fatal("md.Convert: %s", err)
	}

	metaData := meta.Get(context)
	//for s := range metaData {
	//	fmt.Printf("%s %s %T\n", s, metaData[s], metaData[s])
	//}
	//
	//// 4. 获取 HTML 内容
	//fmt.Println("--- HTML 内容 ---")
	//fmt.Println(buf.String())

	result := helper.FlattenTOC(buf.String(), "ol", "class")

	tocHTML := ""
	bodyHTML := result
	if includeTOC {
		tocHTML = ExtractTOCHTML(result)
		bodyHTML = StripTOCHTML(result)
	}

	return &MarkdownResult{
		Meta:     metaData,
		Markdown: GetMarkdownOnly(text),
		HTML:     bodyHTML,
		TOCHTML:  tocHTML,
	}
}

func annotateFenceLanguages(input string) string {
	if !strings.Contains(input, "```") {
		return input
	}
	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines))

	inFence := false
	fenceStartLine := ""
	fenceContent := make([]string, 0, 16)

	flushFence := func() {
		if fenceStartLine == "" {
			return
		}
		opening := fenceStartLine
		if lang := guessFenceLanguage(strings.Join(fenceContent, "\n")); lang != "" {
			opening = "```" + lang
		}
		out = append(out, opening)
		out = append(out, fenceContent...)
		out = append(out, "```")
		fenceStartLine = ""
		fenceContent = fenceContent[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if trimmed == "```" {
				inFence = true
				fenceStartLine = line
				fenceContent = fenceContent[:0]
				continue
			}
			out = append(out, line)
			continue
		}

		if trimmed == "```" {
			flushFence()
			inFence = false
			continue
		}
		fenceContent = append(fenceContent, line)
	}

	if inFence {
		out = append(out, fenceStartLine)
		out = append(out, fenceContent...)
	}

	return strings.Join(out, "\n")
}

func guessFenceLanguage(content string) string {
	c := strings.TrimSpace(content)
	if c == "" {
		return ""
	}
	lower := strings.ToLower(c)

	switch {
	case strings.HasPrefix(lower, "#!/bin/bash"), strings.HasPrefix(lower, "#!/usr/bin/env bash"):
		return "bash"
	case strings.HasPrefix(lower, "#!/bin/sh"), strings.HasPrefix(lower, "#!/usr/bin/env sh"):
		return "sh"
	case strings.Contains(lower, "package main"), strings.Contains(lower, "func main("):
		return "go"
	case strings.Contains(lower, "fmt.println("), strings.Contains(lower, "func "):
		return "go"
	case strings.Contains(lower, "import ") && strings.Contains(lower, " from "):
		return "javascript"
	case strings.Contains(lower, "console.log("), strings.Contains(lower, "function "):
		return "javascript"
	case strings.Contains(lower, "def ") && strings.Contains(lower, ":\n"):
		return "python"
	case strings.Contains(lower, "select ") && strings.Contains(lower, " from "):
		return "sql"
	case strings.HasPrefix(c, "{") || strings.HasPrefix(c, "["):
		if strings.Contains(c, "\"") && strings.Contains(c, ":") {
			return "json"
		}
	case strings.Contains(lower, "<html"), strings.Contains(lower, "<div"), strings.Contains(lower, "<body"):
		return "html"
	case strings.Contains(lower, "fn main("):
		return "rust"
	}

	return ""
}
