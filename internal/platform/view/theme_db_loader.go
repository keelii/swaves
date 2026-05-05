package view

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/shared/share"

	"github.com/gofiber/fiber/v3"
	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
)

func NewThemeDBViewEngineWithShared(model *db.DB, sharedDir string, reload bool) (fiber.Views, func(app *fiber.App)) {
	return newThemeDBViewEngine(model, sharedDir, nil, reload)
}

func NewThemeDBViewEngineWithSharedFS(model *db.DB, sharedFS fs.FS, reload bool) (fiber.Views, func(app *fiber.App)) {
	return newThemeDBViewEngine(model, "", sharedFS, reload)
}

func newThemeDBViewEngine(model *db.DB, sharedDir string, sharedFS fs.FS, reload bool) (fiber.Views, func(app *fiber.App)) {
	urlForStore := share.NewURLForStore()
	view := newMiniJinjaViewWithLoader(
		newCurrentThemeTemplateLoader(model, sharedDir, sharedFS),
		func() ([]string, error) {
			return collectCurrentThemeTemplateNames(model, sharedDir, sharedFS)
		},
		reload,
	)
	registerViewFunc(view.env, urlForStore.ResolveURL)
	initURLResolver := func(app *fiber.App) {
		urlForStore.SetResolver(newURLForResolver(app))
	}
	return view, initURLResolver
}

func newCurrentThemeTemplateLoader(model *db.DB, sharedDir string, sharedFS fs.FS) minijinja.LoaderFunc {
	return func(name string) (string, error) {
		normalizedName, err := normalizeTemplateName(name)
		if err != nil {
			return "", err
		}
		// In development prefer disk/shared templates over DB-stored ones so
		// developers can iterate on files in web/templates without DB overrides.
		if config.IsDevelopment {
			// In dev, prefer theme-specific disk file under web/templates/themes/<code>/<name>
			// then prefer shared root, then fall back to DB.
			if theme, errTheme := db.GetCurrentTheme(model); errTheme == nil {
				themeCandidate := filepath.ToSlash(filepath.Join("themes", theme.Code, normalizedName))
				content, ok, err := readSharedTemplate(themeCandidate, sharedDir, sharedFS)
				if err != nil {
					return "", err
				}
				if ok {
					return content, nil
				}
			}

			// try shared root
			content, ok, err := readSharedTemplate(normalizedName, sharedDir, sharedFS)
			if err != nil {
				return "", err
			}
			if ok {
				return content, nil
			}

			// fall back to DB if shared template not present
			content, ok, err = readCurrentThemeTemplate(model, normalizedName)
			if err != nil {
				return "", err
			}
			if ok {
				return content, nil
			}
			return "", minijinja.NewError(minijinja.ErrTemplateNotFound, normalizedName)
		}

		// Production/default behavior: prefer DB (current theme) and fall back to shared disk
		content, ok, err := readCurrentThemeTemplate(model, normalizedName)
		if err != nil {
			return "", err
		}
		if ok {
			return content, nil
		}

		content, ok, err = readSharedTemplate(normalizedName, sharedDir, sharedFS)
		if err != nil {
			return "", err
		}
		if ok {
			return content, nil
		}
		return "", minijinja.NewError(minijinja.ErrTemplateNotFound, normalizedName)
	}
}

func readCurrentThemeTemplate(model *db.DB, normalizedName string) (string, bool, error) {
	theme, err := db.GetCurrentTheme(model)
	if err != nil {
		if db.IsErrNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load current theme failed: %w", err)
	}

	files, err := parseThemeCacheFiles(theme.Files)
	if err != nil {
		return "", false, fmt.Errorf("parse current theme files failed: %w", err)
	}

	content, ok := files[normalizedName]
	return content, ok, nil
}

func collectCurrentThemeTemplateNames(model *db.DB, sharedDir string, sharedFS fs.FS) ([]string, error) {
	nameSet := map[string]struct{}{}

	theme, err := db.GetCurrentTheme(model)
	if err != nil && !db.IsErrNotFound(err) {
		return nil, fmt.Errorf("load current theme failed: %w", err)
	}
	if err == nil {
		files, err := parseThemeCacheFiles(theme.Files)
		if err != nil {
			return nil, fmt.Errorf("parse current theme files failed: %w", err)
		}
		for name := range files {
			nameSet[name] = struct{}{}
		}
	}

	sharedNames, err := collectSharedTemplateNames(sharedDir, sharedFS)
	if err != nil {
		return nil, err
	}
	for _, name := range sharedNames {
		nameSet[name] = struct{}{}
	}

	return sortedTemplateNames(nameSet), nil
}

func collectSharedTemplateNames(sharedDir string, sharedFS fs.FS) ([]string, error) {
	if sharedFS != nil {
		nameSet := map[string]struct{}{}
		err := fs.WalkDir(sharedFS, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(entry.Name()) != ".html" {
				return nil
			}
			nameSet[filepath.ToSlash(filePath)] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return sortedTemplateNames(nameSet), nil
	}

	if sharedDir == "" {
		return nil, nil
	}
	return collectTemplateNamesFromDir(sharedDir)
}

func readSharedTemplate(normalizedName string, sharedDir string, sharedFS fs.FS) (string, bool, error) {
	if sharedFS != nil {
		content, err := fs.ReadFile(sharedFS, normalizedName)
		if err == nil {
			return string(content), true, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}

	if sharedDir == "" {
		return "", false, nil
	}
	return readTemplateFromDir(sharedDir, normalizedName)
}
