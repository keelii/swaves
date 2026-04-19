package db

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	webassets "swaves/web"
)

const (
	DefaultThemeCode       = "tuft"
	legacyDefaultThemeCode = "default-theme-template"
)

func loadDefaultThemeFiles() (map[string]string, error) {
	templateFS := webassets.TemplateFS()
	themeDir := path.Join("themes", DefaultThemeCode)
	entries, err := fs.ReadDir(templateFS, themeDir)
	if err != nil {
		return nil, WrapInternalErr("loadDefaultThemeFiles.ReadDir", err)
	}

	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		content, err := fs.ReadFile(templateFS, path.Join(themeDir, entry.Name()))
		if err != nil {
			return nil, WrapInternalErr("loadDefaultThemeFiles.ReadFile", err)
		}
		files[entry.Name()] = string(content)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("default theme %q has no html files", DefaultThemeCode)
	}
	return files, nil
}

func newDefaultTheme() (*Theme, error) {
	files, err := loadDefaultThemeFiles()
	if err != nil {
		return nil, err
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return nil, WrapInternalErr("newDefaultTheme.Marshal", err)
	}

	nowUnix := time.Now().Unix()
	return &Theme{
		Name:        DefaultThemeCode,
		Code:        DefaultThemeCode,
		Description: "内置默认主题",
		Author:      "swaves",
		Files:       string(filesJSON),
		CurrentFile: "home.html",
		Status:      "draft",
		IsCurrent:   1,
		IsBuiltin:   1,
		Version:     1,
		CreatedAt:   nowUnix,
		UpdatedAt:   nowUnix,
	}, nil
}

func ensureDefaultThemeCurrent(db *DB, theme *Theme) error {
	currentTheme, err := GetCurrentTheme(db)
	if err == nil {
		if currentTheme.ID == theme.ID && theme.IsCurrent != 1 {
			theme.IsCurrent = 1
		}
		return nil
	}
	if !IsErrNotFound(err) {
		return err
	}
	if err := SetThemeCurrent(db, theme.ID); err != nil {
		return WrapInternalErr("EnsureDefaultTheme.SetCurrent", err)
	}
	theme.IsCurrent = 1
	return nil
}

func syncDefaultTheme(db *DB, theme *Theme) error {
	defaultTheme, err := newDefaultTheme()
	if err != nil {
		return err
	}

	if theme.Name == defaultTheme.Name &&
		theme.Description == defaultTheme.Description &&
		theme.Author == defaultTheme.Author &&
		theme.Files == defaultTheme.Files &&
		theme.CurrentFile == defaultTheme.CurrentFile &&
		theme.Status == defaultTheme.Status &&
		theme.IsBuiltin == defaultTheme.IsBuiltin {
		return nil
	}

	nowUnix := time.Now().Unix()
	_, err = db.Exec(
		`UPDATE `+string(TableThemes)+` SET name=?, description=?, author=?, files=?, current_file=?, status=?, is_builtin=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		defaultTheme.Name,
		defaultTheme.Description,
		defaultTheme.Author,
		defaultTheme.Files,
		defaultTheme.CurrentFile,
		defaultTheme.Status,
		defaultTheme.IsBuiltin,
		nowUnix,
		theme.ID,
	)
	if err != nil {
		return WrapInternalErr("EnsureDefaultTheme.Sync", err)
	}
	return ensureDefaultThemeCurrent(db, theme)
}

func EnsureDefaultTheme(db *DB) error {
	theme, err := GetThemeByCode(db, DefaultThemeCode)
	if err == nil {
		return syncDefaultTheme(db, theme)
	}
	if !IsErrNotFound(err) {
		return err
	}

	theme, err = GetThemeByCode(db, legacyDefaultThemeCode)
	if err == nil {
		nowUnix := time.Now().Unix()
		_, updateErr := db.Exec(
			`UPDATE `+string(TableThemes)+` SET code=?, name=?, description=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
			DefaultThemeCode,
			DefaultThemeCode,
			"内置默认主题",
			nowUnix,
			theme.ID,
		)
		if updateErr != nil {
			return WrapInternalErr("EnsureDefaultTheme.RenameLegacy", updateErr)
		}
		theme.Code = DefaultThemeCode
		theme.Name = DefaultThemeCode
		theme.Description = "内置默认主题"
		return syncDefaultTheme(db, theme)
	}
	if !IsErrNotFound(err) {
		return err
	}

	theme, err = newDefaultTheme()
	if err != nil {
		return err
	}
	_, err = GetCurrentTheme(db)
	if err == nil {
		theme.IsCurrent = 0
	} else if !IsErrNotFound(err) {
		return err
	}
	_, err = CreateTheme(db, theme)
	if err != nil {
		return err
	}
	return ensureDefaultThemeCurrent(db, theme)
}
