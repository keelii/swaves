package themefiles

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultCurrentFile = "home.html"

func NormalizePath(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "site/")
	if strings.HasPrefix(name, "themes/") {
		rest := strings.TrimPrefix(name, "themes/")
		if idx := strings.Index(rest, "/"); idx >= 0 {
			name = rest[idx+1:]
		}
	}
	if name == "" {
		return "", false
	}

	switch {
	case strings.HasPrefix(name, "include/"):
		name = "inc_" + strings.TrimPrefix(name, "include/")
	case strings.HasPrefix(name, "macro/"):
		name = "macro_" + strings.TrimPrefix(name, "macro/")
	case strings.HasPrefix(name, "layout/"):
		baseName := strings.TrimPrefix(name, "layout/")
		if baseName == "layout.html" {
			name = "layout_main.html"
		} else {
			name = "layout_" + baseName
		}
	}

	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/") {
		return "", false
	}
	if !strings.HasSuffix(name, ".html") {
		return "", false
	}
	return name, true
}

func ParseJSON(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}

	rawFiles := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &rawFiles); err != nil {
		return nil, err
	}

	files := make(map[string]string, len(rawFiles))
	for rawName, content := range rawFiles {
		name, ok := NormalizePath(rawName)
		if !ok {
			return nil, fmt.Errorf("invalid theme file path: %s", rawName)
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("duplicate theme file path: %s", name)
		}
		files[name] = content
	}
	return files, nil
}

func MarshalJSON(files map[string]string) (string, error) {
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func SortedPaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths
}

func ResolveCurrentFile(files map[string]string, candidates ...string) string {
	for _, candidate := range candidates {
		normalized, ok := NormalizePath(candidate)
		if !ok {
			continue
		}
		if _, exists := files[normalized]; exists {
			return normalized
		}
	}
	if _, ok := files[DefaultCurrentFile]; ok {
		return DefaultCurrentFile
	}
	for _, name := range SortedPaths(files) {
		return name
	}
	return DefaultCurrentFile
}
