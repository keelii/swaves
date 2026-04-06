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
