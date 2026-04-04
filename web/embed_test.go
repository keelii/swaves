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
}
