package md

import (
	"bytes"

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
		if len(img.Title) > 0 {
			w.WriteString("<figure class=\"fullwidth\">")
			w.WriteString(`<img src="` + string(img.Destination) + `" alt="` + string(imageAltText(img, source)) + `">`)
			w.WriteString(`<figcaption>` + string(img.Title) + `</figcaption>`)
		} else {
			w.WriteString(`<p><img src="` + string(img.Destination) + `" alt="` + string(imageAltText(img, source)) + `"></p>`)
		}
	} else {
		if len(img.Title) > 0 {
			w.WriteString("</figure>")
		}
	}
	return ast.WalkContinue, nil
}
