package view

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/themefiles"
	"swaves/internal/shared/pathutil"
)

const (
	runtimeThemeBuiltinCode = db.DefaultThemeCode
	runtimeThemeIncludeDir  = "include"
)

var runtimeThemeSharedFiles = []string{
	"include/favicon.html",
	"include/math.html",
}

func ResolveThemeCacheRoot(_ string) (string, error) {
	cacheRoot, err := pathutil.ResolveProcessCachePath("themes")
	if err != nil {
		return "", fmt.Errorf("resolve theme cache root failed: %w", err)
	}
	return cacheRoot, nil
}

func MaterializeCurrentThemeCache(model *db.DB, sqliteFile string, templateRoot string, templateFS fs.FS) (string, error) {
	theme, err := db.GetCurrentTheme(model)
	if err != nil {
		return "", err
	}

	dirName, ok := normalizeThemeCacheDirName(theme.Code)
	if !ok {
		return "", fmt.Errorf("invalid theme code %q", theme.Code)
	}
	files, err := parseThemeCacheFiles(theme.Files)
	if err != nil {
		return "", fmt.Errorf("parse current theme files failed: %w", err)
	}

	cacheRoot, err := ResolveThemeCacheRoot(sqliteFile)
	if err != nil {
		return "", err
	}
	targetRoot := filepath.Join(cacheRoot, dirName)
	if err := resetThemeCacheRoot(cacheRoot, targetRoot); err != nil {
		return "", err
	}
	if err := writeThemeFiles(targetRoot, files); err != nil {
		return "", err
	}
	if err := copySharedSiteFiles(targetRoot, templateRoot, templateFS); err != nil {
		return "", err
	}
	logger.Info("[theme] loaded: code=%s source=db files=%d root=%s", theme.Code, len(files), targetRoot)
	return targetRoot, nil
}

func MaterializeBuiltinThemeCache(sqliteFile string, templateRoot string, templateFS fs.FS) (string, error) {
	files, err := loadBuiltinThemeFiles(templateRoot, templateFS, runtimeThemeBuiltinCode)
	if err != nil {
		return "", err
	}

	cacheRoot, err := ResolveThemeCacheRoot(sqliteFile)
	if err != nil {
		return "", err
	}
	targetRoot := filepath.Join(cacheRoot, runtimeThemeBuiltinCode)
	if err := resetThemeCacheRoot(cacheRoot, targetRoot); err != nil {
		return "", err
	}
	if err := writeThemeFiles(targetRoot, files); err != nil {
		return "", err
	}
	if err := copySharedSiteFiles(targetRoot, templateRoot, templateFS); err != nil {
		return "", err
	}
	logger.Info("[theme] loaded: code=%s source=builtin files=%d root=%s", runtimeThemeBuiltinCode, len(files), targetRoot)
	return targetRoot, nil
}

func resetThemeCacheRoot(cacheRoot string, targetRoot string) error {
	if err := os.RemoveAll(cacheRoot); err != nil {
		return fmt.Errorf("clear theme cache root failed: %w", err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create theme cache directory failed: %w", err)
	}
	return nil
}

func writeThemeFiles(targetRoot string, files map[string]string) error {
	for name, content := range files {
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(name))
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write theme file %q failed: %w", name, err)
		}
	}
	return nil
}

func copySharedSiteFiles(targetRoot string, templateRoot string, templateFS fs.FS) error {
	includeRoot := filepath.Join(targetRoot, runtimeThemeIncludeDir)
	if err := os.MkdirAll(includeRoot, 0o755); err != nil {
		return fmt.Errorf("create theme include directory failed: %w", err)
	}
	for _, name := range runtimeThemeSharedFiles {
		content, err := readTemplateSourceFile(templateRoot, templateFS, name)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(name))
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write shared theme file %q failed: %w", name, err)
		}
	}
	return nil
}

func loadBuiltinThemeFiles(templateRoot string, templateFS fs.FS, themeCode string) (map[string]string, error) {
	themeDir := path.Join("themes", themeCode)
	names, err := listTemplateSourceFiles(templateRoot, templateFS, themeDir)
	if err != nil {
		return nil, err
	}

	files := make(map[string]string, len(names))
	for _, name := range names {
		if strings.Contains(name, "/") {
			continue
		}
		content, err := readTemplateSourceFile(templateRoot, templateFS, path.Join(themeDir, name))
		if err != nil {
			return nil, err
		}
		files[name] = content
	}
	return files, nil
}

func listTemplateSourceFiles(templateRoot string, templateFS fs.FS, dirName string) ([]string, error) {
	if templateFS != nil {
		entries, err := fs.ReadDir(templateFS, dirName)
		if err != nil {
			return nil, fmt.Errorf("read embedded template directory %q failed: %w", dirName, err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
				continue
			}
			names = append(names, entry.Name())
		}
		return names, nil
	}

	entries, err := os.ReadDir(filepath.Join(templateRoot, dirName))
	if err != nil {
		return nil, fmt.Errorf("read template directory %q failed: %w", dirName, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func readTemplateSourceFile(templateRoot string, templateFS fs.FS, name string) (string, error) {
	if templateFS != nil {
		content, err := fs.ReadFile(templateFS, filepath.ToSlash(name))
		if err != nil {
			return "", fmt.Errorf("read embedded template %q failed: %w", name, err)
		}
		return string(content), nil
	}

	content, err := os.ReadFile(filepath.Join(templateRoot, filepath.FromSlash(name)))
	if err != nil {
		return "", fmt.Errorf("read template %q failed: %w", name, err)
	}
	return string(content), nil
}

func parseThemeCacheFiles(raw string) (map[string]string, error) {
	return themefiles.ParseJSON(raw)
}

func normalizeThemeCacheDirName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.ToSlash(name)
	if name == "" {
		return "", false
	}
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}
