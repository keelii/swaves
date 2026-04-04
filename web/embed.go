package webassets

import (
	"embed"
	"io/fs"
)

//go:embed static templates
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
