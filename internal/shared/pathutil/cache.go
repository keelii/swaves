package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveProcessCacheRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve process cache root failed: %w", err)
	}
	wd = strings.TrimSpace(wd)
	if wd == "" {
		return "", fmt.Errorf("resolve process cache root failed: empty working directory")
	}
	return filepath.Join(wd, ".cache"), nil
}

func ResolveProcessCachePath(parts ...string) (string, error) {
	root, err := ResolveProcessCacheRoot()
	if err != nil {
		return "", err
	}
	segments := []string{root}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return filepath.Join(segments...), nil
}
