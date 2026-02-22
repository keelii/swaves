package main

import "testing"

func TestMiniJinjaViewLoadTemplates(t *testing.T) {
	view, _, _ := NewViewEngine("./web/templates", false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}
}
