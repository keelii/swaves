package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	themeInputFlag   = flag.String("input", "", "path to theme json payload file")
	themeOutRootFlag = flag.String("out", filepath.Join("web", "templates", "themes"), "output root directory")
)

type themeTransferPayload struct {
	Name        string            `json:"name"`
	Code        string            `json:"code"`
	Files       map[string]string `json:"files"`
	CurrentFile string            `json:"current_file"`
}

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s --input theme.json [--out web/templates/themes]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(out, "Extract a theme import JSON payload into local template source files.")
		fmt.Fprintln(out, "The tool writes files into <out>/<themeCode>/ and overwrites files present in JSON.")
		fmt.Fprintln(out, "Only .html files are written; other files in payload are skipped with warnings.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Examples:")
		fmt.Fprintln(out, "  go run ./cmd/theme_extract --input /tmp/theme.json")
		fmt.Fprintln(out, "  go run ./cmd/theme_extract --input /tmp/theme.json --out ./web/templates/themes")
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	inputPath := strings.TrimSpace(*themeInputFlag)
	if inputPath == "" {
		if flag.NArg() > 0 {
			inputPath = strings.TrimSpace(flag.Arg(0))
		}
	}
	if inputPath == "" {
		exitWithError(errors.New("input path is required"))
	}

	outRoot := strings.TrimSpace(*themeOutRootFlag)
	if outRoot == "" {
		exitWithError(errors.New("output root is required"))
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		exitWithError(fmt.Errorf("read input file failed: %w", err))
	}

	payload, err := decodeThemeTransferPayload(raw)
	if err != nil {
		exitWithError(fmt.Errorf("decode input theme payload failed: %w", err))
	}

	themeCode, err := resolveThemeCode(payload)
	if err != nil {
		exitWithError(err)
	}

	written, skipped, err := extractThemeFiles(payload.Files, outRoot, themeCode, os.Stderr)
	if err != nil {
		exitWithError(err)
	}

	targetDir := filepath.Join(outRoot, themeCode)
	fmt.Printf("extracted theme %q to %s (%d files written, %d skipped)\n", themeCode, targetDir, written, skipped)
}

func decodeThemeTransferPayload(raw []byte) (*themeTransferPayload, error) {
	var envelope struct {
		Name        string          `json:"name"`
		Code        string          `json:"code"`
		Files       json.RawMessage `json:"files"`
		CurrentFile string          `json:"current_file"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Files) == 0 {
		return nil, fmt.Errorf("theme files is required")
	}

	files, err := decodeThemeTransferFiles(envelope.Files)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("theme files is empty")
	}

	return &themeTransferPayload{
		Name:        strings.TrimSpace(envelope.Name),
		Code:        strings.TrimSpace(envelope.Code),
		CurrentFile: strings.TrimSpace(envelope.CurrentFile),
		Files:       files,
	}, nil
}

func decodeThemeTransferFiles(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("theme files is required")
	}

	if raw[0] == '"' {
		var filesJSON string
		if err := json.Unmarshal(raw, &filesJSON); err != nil {
			return nil, err
		}

		files := map[string]string{}
		if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
			return nil, err
		}
		return files, nil
	}

	files := map[string]string{}
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func resolveThemeCode(payload *themeTransferPayload) (string, error) {
	if payload == nil {
		return "", fmt.Errorf("theme payload is required")
	}
	code := strings.TrimSpace(payload.Code)
	if code == "" {
		code = strings.TrimSpace(payload.Name)
	}
	if code == "" {
		return "", fmt.Errorf("theme code or name is required")
	}

	clean := path.Clean(filepath.ToSlash(code))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return "", fmt.Errorf("invalid theme code: %s", code)
	}
	return clean, nil
}

func extractThemeFiles(files map[string]string, outRoot string, themeCode string, warnOut io.Writer) (int, int, error) {
	if len(files) == 0 {
		return 0, 0, fmt.Errorf("theme files is empty")
	}
	if strings.TrimSpace(outRoot) == "" {
		return 0, 0, fmt.Errorf("output root is required")
	}
	if strings.TrimSpace(themeCode) == "" {
		return 0, 0, fmt.Errorf("theme code is required")
	}

	themeRoot := filepath.Join(outRoot, themeCode)
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)

	written := 0
	skipped := 0
	for _, rawName := range paths {
		normalizedName, err := normalizeExtractFilePath(rawName)
		if err != nil {
			return written, skipped, err
		}
		if !strings.HasSuffix(strings.ToLower(normalizedName), ".html") {
			skipped++
			if warnOut != nil {
				fmt.Fprintf(warnOut, "skip non-html file: %s\n", rawName)
			}
			continue
		}

		targetPath := filepath.Join(themeRoot, filepath.FromSlash(normalizedName))
		rel, err := filepath.Rel(themeRoot, targetPath)
		if err != nil {
			return written, skipped, fmt.Errorf("build target path failed for %q: %w", rawName, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			return written, skipped, fmt.Errorf("invalid theme file path: %s", rawName)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return written, skipped, fmt.Errorf("create directory for %q failed: %w", normalizedName, err)
		}
		if err := os.WriteFile(targetPath, []byte(files[rawName]), 0o644); err != nil {
			return written, skipped, fmt.Errorf("write theme file %q failed: %w", normalizedName, err)
		}
		written++
	}

	if written == 0 {
		return 0, skipped, fmt.Errorf("no html theme files found in payload")
	}
	return written, skipped, nil
}

func normalizeExtractFilePath(name string) (string, error) {
	rawName := strings.TrimSpace(name)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("invalid theme file path: %s", name)
	}
	name = filepath.ToSlash(name)
	if len(name) >= 2 && name[1] == ':' {
		return "", fmt.Errorf("invalid theme file path: %s", rawName)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid theme file path: %s", rawName)
	}
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "site/")
	if strings.HasPrefix(name, "themes/") {
		parts := strings.Split(name, "/")
		if len(parts) >= 3 {
			name = strings.Join(parts[2:], "/")
		}
	}

	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid theme file path: %s", rawName)
		}
	}

	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid theme file path: %s", rawName)
	}
	return name, nil
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "theme extract failed: %v\n", err)
	os.Exit(1)
}
