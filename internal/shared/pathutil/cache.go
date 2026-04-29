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

func EnsureProcessCacheDir(parts ...string) (string, error) {
	path, err := ResolveProcessCachePath(parts...)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("ensure process cache dir failed: %w", err)
	}
	return path, nil
}

func CreateProcessCacheTempDir(prefix string, parts ...string) (string, error) {
	base, err := EnsureProcessCacheDir(parts...)
	if err != nil {
		return "", err
	}
	tempDir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return "", fmt.Errorf("create process cache temp dir failed: %w", err)
	}
	return tempDir, nil
}

func ValidateProcessCachePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("validate process cache path failed: empty path")
	}

	root, err := ResolveProcessCacheRoot()
	if err != nil {
		return err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("validate process cache path failed: resolve cache root failed: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("validate process cache path failed: resolve path failed: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return fmt.Errorf("validate process cache path failed: compare path failed: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("validate process cache path failed: %s is outside %s", pathAbs, rootAbs)
	}
	return nil
}
