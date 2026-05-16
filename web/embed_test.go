package webassets

import (
	"io/fs"
	"testing"
)

func TestTemplateFSIncludesTemplates(t *testing.T) {
	if _, err := fs.ReadFile(TemplateFS(), "sui/layout/base.html"); err != nil {
		t.Fatalf("read embedded template failed: %v", err)
	}
}

func TestStaticFSIncludesStaticFiles(t *testing.T) {
	if _, err := fs.ReadFile(StaticFS(), "sui/sui.css"); err != nil {
		t.Fatalf("read embedded static file failed: %v", err)
	}
	if _, err := fs.ReadFile(StaticFS(), "favicon.svg"); err != nil {
		t.Fatalf("read embedded favicon failed: %v", err)
	}
	if _, err := fs.ReadFile(StaticFS(), "seditor/dist/seditor.min.js"); err != nil {
		t.Fatalf("read embedded minified editor asset failed: %v", err)
	}
	if _, err := fs.ReadFile(StaticFS(), "katex/katex.min.js"); err != nil {
		t.Fatalf("read embedded minified katex asset failed: %v", err)
	}
	if _, err := fs.ReadFile(StaticFS(), "svg-pan-zoom/svg-pan-zoom.min.js"); err != nil {
		t.Fatalf("read embedded svg-pan-zoom asset failed: %v", err)
	}
	for _, name := range []string{
		"dash/layout-shared.js",
		"dash/post-editor.js",
		"dash/import.js",
		"dash/themes-index.js",
		"dash/redirects-index.js",
		"dash/categories-tree.js",
		"dash/settings-system-update.js",
		"site/mermaid-init.js",
		"site/like-action.js",
		"sui/post-edit.js",
	} {
		if _, err := fs.ReadFile(StaticFS(), name); err != nil {
			t.Fatalf("read embedded extracted js asset %q failed: %v", name, err)
		}
	}
}

func TestStaticFSExcludesUncompressedDuplicateAssets(t *testing.T) {
	excluded := []string{
		"dash/tex-chtml.js",
		"katex/README.md",
		"katex/katex.js",
		"seditor/dist/seditor.js",
		"site/tufte-css/tufte.css",
	}

	for _, name := range excluded {
		if _, err := fs.Stat(StaticFS(), name); err == nil {
			t.Fatalf("expected embedded static fs to exclude %q", name)
		}
	}
}
