package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"swaves/internal/platform/db"
)

var (
	sqliteFileFlag  = flag.String("sqlite", "", "target sqlite database path")
	themeCodeFlag   = flag.String("theme", db.DefaultThemeCode, "theme code to write into themes table")
	themeDirFlag    = flag.String("dir", "", "local theme source directory (defaults to web/templates/themes/<theme>)")
	themeAuthorFlag = flag.String("author", "swaves", "theme author saved into database")
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s --sqlite data.sqlite [--theme tuft] [--dir web/templates/themes/tuft] [--author swaves]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(out, "Sync local flat theme source files into the database themes table.")
		fmt.Fprintln(out, "The source directory must only contain top-level *.html files.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Examples:")
		fmt.Fprintln(out, "  go run ./cmd/theme_sync --sqlite data.sqlite --theme tuft")
		fmt.Fprintln(out, "  go run ./cmd/theme_sync --sqlite data.sqlite --theme my-theme --dir web/templates/themes/my-theme --author keelii")
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	sqliteFile := strings.TrimSpace(*sqliteFileFlag)
	if sqliteFile == "" {
		exitWithError(errors.New("sqlite path is required"))
	}

	themeCode := strings.TrimSpace(*themeCodeFlag)
	if themeCode == "" {
		exitWithError(errors.New("theme code is required"))
	}

	themeDir := strings.TrimSpace(*themeDirFlag)
	if themeDir == "" {
		themeDir = filepath.Join("web", "templates", "themes", themeCode)
	}

	files, err := loadLocalThemeFiles(themeDir)
	if err != nil {
		exitWithError(err)
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		exitWithError(fmt.Errorf("marshal theme files failed: %w", err))
	}

	model := db.Open(db.Options{DSN: sqliteFile})
	defer func() {
		_ = model.Close()
	}()

	nowUnix := time.Now().Unix()
	theme, err := db.GetThemeByCode(model, themeCode)
	if err != nil {
		if !db.IsErrNotFound(err) {
			exitWithError(err)
		}

		currentFile := resolveThemeCurrentFile(files, "")
		createdTheme := &db.Theme{
			Name:        themeCode,
			Code:        themeCode,
			Description: "synced from local theme source",
			Author:      strings.TrimSpace(*themeAuthorFlag),
			Files:       string(filesJSON),
			CurrentFile: currentFile,
			Status:      "published",
			IsBuiltin:   boolToInt(themeCode == db.DefaultThemeCode),
			Version:     1,
			CreatedAt:   nowUnix,
			UpdatedAt:   nowUnix,
		}
		if _, err := db.CreateTheme(model, createdTheme); err != nil {
			exitWithError(err)
		}
		if _, currentErr := db.GetCurrentTheme(model); db.IsErrNotFound(currentErr) {
			if err := db.SetThemeCurrent(model, createdTheme.ID); err != nil {
				exitWithError(err)
			}
		} else if currentErr != nil {
			exitWithError(currentErr)
		}

		fmt.Printf("synced local theme source %q from %s into %s\n", themeCode, themeDir, sqliteFile)
		return
	}

	theme.Name = themeCode
	if strings.TrimSpace(*themeAuthorFlag) != "" {
		theme.Author = strings.TrimSpace(*themeAuthorFlag)
	}
	if strings.TrimSpace(theme.Description) == "" {
		theme.Description = "synced from local theme source"
	}
	theme.Files = string(filesJSON)
	theme.CurrentFile = resolveThemeCurrentFile(files, theme.CurrentFile)
	if strings.TrimSpace(theme.Status) == "" {
		theme.Status = "published"
	}
	if themeCode == db.DefaultThemeCode {
		theme.IsBuiltin = 1
	}
	if theme.Version <= 0 {
		theme.Version = 1
	}
	theme.UpdatedAt = nowUnix
	if err := db.UpdateTheme(model, theme, theme.Version); err != nil {
		exitWithError(err)
	}

	fmt.Printf("synced local theme source %q from %s into %s\n", themeCode, themeDir, sqliteFile)
}

func loadLocalThemeFiles(themeDir string) (map[string]string, error) {
	entries, err := os.ReadDir(themeDir)
	if err != nil {
		return nil, fmt.Errorf("read theme directory failed: %w", err)
	}

	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("theme directory must be flat, found subdirectory %q", entry.Name())
		}
		if !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(themeDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read theme file %q failed: %w", entry.Name(), err)
		}
		files[entry.Name()] = string(content)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no html theme files found in %s", themeDir)
	}
	return files, nil
}

func resolveThemeCurrentFile(files map[string]string, currentFile string) string {
	currentFile = strings.TrimSpace(currentFile)
	if currentFile != "" {
		if _, ok := files[currentFile]; ok {
			return currentFile
		}
	}
	if _, ok := files["home.html"]; ok {
		return "home.html"
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "home.html"
	}
	return names[0]
}

func boolToInt(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "theme sync failed: %v\n", err)
	os.Exit(1)
}
