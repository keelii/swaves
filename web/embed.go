package webassets

import (
	"embed"
	"io/fs"
)

// Embed only production runtime assets. Local development still serves from
// disk when template reload is enabled, so uncompressed/dev-only assets do not
// need to ship inside the release binary.
//
//go:embed templates
//go:embed static/favicon.svg
//go:embed static/ceditor/ceditor.css
//go:embed static/ceditor/dist/ceditor.min.js
//go:embed static/dash/main.js
//go:embed static/dash/style.css
//go:embed static/robots.txt
//go:embed static/site/main.js
//go:embed static/site/style.css
//go:embed static/site/tufte-css/tufte.min.css
//go:embed static/seditor/dist/seditor.min.js
//go:embed static/sui
//go:embed static/katex/katex.min.css
//go:embed static/katex/katex.min.js
//go:embed static/katex/contrib/auto-render.min.js
var embeddedFiles embed.FS

func StaticFS() fs.FS {
	filesystem, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	return filesystem
}

func TemplateFS() fs.FS {
	filesystem, err := fs.Sub(embeddedFiles, "templates")
	if err != nil {
		panic(err)
	}
	return filesystem
}
