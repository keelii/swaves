package view

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestZZMapIndexNumericKey(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`A={{ PostUVMap[PostID ~ ""] }}; B={{ PostUVMap["12"] }}`), 0o644); err != nil {
		t.Fatal(err)
	}
	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string { return name })
	if err := view.Load(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := view.Render(&out, "page", map[string]any{"PostID": int64(12), "PostUVMap": map[int64]int{12: 5}})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(out.String())
}
