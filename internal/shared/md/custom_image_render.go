package md

import (
	"bytes"
	"fmt"
	"html"
	"swaves/internal/platform/logger"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

type FigureRenderer struct{}

func (r *FigureRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

func imageAltText(img *ast.Image, source []byte) string {
	var buf bytes.Buffer
	for n := img.FirstChild(); n != nil; n = n.NextSibling() {
		if t, ok := n.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
	return buf.String()
}
func (r *FigureRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	img := node.(*ast.Image)

	if entering {
		alt := html.EscapeString(imageAltText(img, source))
		src := html.EscapeString(string(img.Destination))

		if len(img.Title) > 0 {
			title := html.EscapeString(string(img.Title))
			_, err := fmt.Fprintf(w,
				`<figure class="fullwidth"><img src="%s" alt="%s"><figcaption>%s</figcaption>`,
				src, alt, title)
			if err != nil {
				logger.Error("render image with caption failed: %v", err)
				return ast.WalkStop, err
			}
		} else {
			_, err := fmt.Fprintf(w, `<p><img src="%s" alt="%s"></p>`, src, alt)
			if err != nil {
				logger.Error("render image failed: %v", err)
				return ast.WalkStop, err
			}
		}

		// 关键：返回 SkipChildren，防止 alt 文本子节点被重复渲染
		return ast.WalkSkipChildren, nil
	}

	// exiting 时关闭 figure
	if len(img.Title) > 0 {
		if _, err := w.WriteString("</figure>"); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}
