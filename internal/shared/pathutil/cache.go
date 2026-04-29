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
	return joinCachePath(root, parts...), nil
}

func ResolveDatabaseCacheRoot(sqliteFile string) (string, error) {
	sqliteFile = strings.TrimSpace(sqliteFile)
	if sqliteFile == "" {
		return "", fmt.Errorf("resolve database cache root failed: sqlite file is required")
	}
	absPath, err := filepath.Abs(sqliteFile)
	if err != nil {
		return "", fmt.Errorf("resolve database cache root failed: %w", err)
	}
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return "", fmt.Errorf("resolve database cache root failed: empty sqlite path")
	}
	return filepath.Join(filepath.Dir(absPath), ".cache"), nil
}

func ResolveDatabaseCachePath(sqliteFile string, parts ...string) (string, error) {
	root, err := ResolveDatabaseCacheRoot(sqliteFile)
	if err != nil {
		return "", err
	}
	return joinCachePath(root, parts...), nil
}

func EnsureDatabaseCacheRoot(sqliteFile string) (string, error) {
	root, err := ResolveDatabaseCacheRoot(sqliteFile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("create database cache root failed: %w", err)
	}
	return root, nil
}

func joinCachePath(root string, parts ...string) string {
	segments := []string{root}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return filepath.Join(segments...)
}
