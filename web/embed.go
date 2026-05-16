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
//go:embed static/dash/assets-index.js
//go:embed static/dash/backup-restore.js
//go:embed static/dash/categories-tree.js
//go:embed static/dash/dash-home.js
//go:embed static/dash/encrypted-post-expiry-status.js
//go:embed static/dash/import.js
//go:embed static/dash/install.js
//go:embed static/dash/layout-multiselect.js
//go:embed static/dash/layout-shared.js
//go:embed static/dash/main.js
//go:embed static/dash/monitor.js
//go:embed static/dash/post-editor.js
//go:embed static/dash/post-search.js
//go:embed static/dash/redirects-form.js
//go:embed static/dash/settings-all.js
//go:embed static/dash/settings-form.js
//go:embed static/dash/settings-system-update.js
//go:embed static/dash/style.css
//go:embed static/dash/tasks-form.js
//go:embed static/dash/themes-edit.js
//go:embed static/dash/themes-index.js
//go:embed static/dash/redirects-index.js
//go:embed static/dash/trash-index.js
//go:embed static/robots.txt
//go:embed static/site/comment-reply.js
//go:embed static/site/content-assets.js
//go:embed static/site/like-action.js
//go:embed static/site/main.js
//go:embed static/site/mermaid-init.js
//go:embed static/site/style.css
//go:embed static/site/tufte-css/tufte.min.css
//go:embed static/seditor/dist/seditor.min.js
//go:embed static/sui
//go:embed static/katex/katex.min.css
//go:embed static/katex/katex.min.js
//go:embed static/katex/contrib/auto-render.min.js
//go:embed static/katex/fonts
//go:embed static/mermaid/mermaid.min.js
//go:embed static/svg-pan-zoom/svg-pan-zoom.min.js
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
