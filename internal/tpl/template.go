package tpl

import (
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TplManager 管理所有模板
type TplManager struct {
	templates map[string]*template.Template
	mu        sync.RWMutex
}

var TM = &TplManager{
	templates: make(map[string]*template.Template),
}

// LoadTemplatesDir 自动扫描目录，加载所有 .html 模板
func LoadTemplatesDir(dir string) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".html") {
			return nil
		}

		tmplName := strings.TrimSuffix(info.Name(), ".html") // 模板名字取文件名
		tmpl, err := template.ParseFiles(path)
		if err != nil {
			return err
		}

		TM.mu.Lock()
		TM.templates[tmplName] = tmpl
		TM.mu.Unlock()
		return nil
	})
	if err != nil {
		log.Fatalf("load templates failed: %v", err)
	}
	log.Printf("loaded %d templates from %s", len(TM.templates), dir)
}

// Render 渲染模板
func Render(w io.Writer, name string, data any) error {
	TM.mu.RLock()
	tmpl, ok := TM.templates[name]
	TM.mu.RUnlock()
	if !ok {
		return ErrTemplateNotFound{name}
	}
	return tmpl.Execute(w, data)
}

type ErrTemplateNotFound struct{ Name string }

func (e ErrTemplateNotFound) Error() string {
	return "template not found: " + e.Name
}
