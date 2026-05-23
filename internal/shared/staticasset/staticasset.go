package staticasset

import (
	"path"
	"strings"
)

const (
	VendoredCacheControl = "public, max-age=31536000, immutable"
	AppCacheControl      = "public, max-age=86400"
)

var vendoredStaticPrefixes = []string{
	"/static/katex/",
	"/static/mermaid/",
	"/static/svg-pan-zoom/",
	//"/static/site/tufte-css/",
}

func IsVendored(pathValue string) bool {
	p := normalizePath(pathValue)
	for _, prefix := range vendoredStaticPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func ShouldUseBuildVersion(pathValue string) bool {
	return !IsVendored(pathValue)
}

func normalizePath(pathValue string) string {
	p := strings.TrimSpace(pathValue)
	if p == "" {
		return ""
	}
	if idx := strings.IndexAny(p, "?#"); idx >= 0 {
		p = p[:idx]
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}
