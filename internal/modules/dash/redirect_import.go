package dash

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"swaves/internal/platform/db"
)

var redirectImportHeaders = []string{"from", "to", "status", "enabled"}

func validateRedirectStatus(status int) error {
	if status != 301 && status != 302 {
		return errors.New("status must be 301 or 302")
	}
	return nil
}

func validateRedirectEnabled(enabled int) error {
	if enabled != 0 && enabled != 1 {
		return errors.New("enabled must be 0 or 1")
	}
	return nil
}

func validateRedirectCreateInput(dbx *db.DB, in CreateRedirectInput) (CreateRedirectInput, error) {
	in.From = normalizeRedirectPath(in.From)
	in.To = normalizeRedirectPath(in.To)

	if in.From == "" || in.To == "" {
		return in, errors.New("from and to required")
	}

	conflictPath, conflictSlug, err := findPostRedirectSourceConflicts(dbx, in.From)
	if err != nil {
		return in, err
	}
	if conflictPath != "" {
		return in, fmt.Errorf("来源路径与已发布内容地址冲突：%s", conflictPath)
	}
	if conflictSlug != "" {
		return in, fmt.Errorf("来源路径与文章 slug 冲突：%s", conflictSlug)
	}

	if in.From == in.To {
		return in, errors.New("from 和 to 不能相同")
	}

	if err := validateRedirectStatus(in.Status); err != nil {
		return in, err
	}
	if err := validateRedirectEnabled(in.Enabled); err != nil {
		return in, err
	}

	if _, err := db.GetRedirectByFrom(dbx, in.From); err == nil {
		return in, fmt.Errorf("来源路径已存在重定向：%s", in.From)
	} else if !db.IsErrNotFound(err) {
		return in, err
	}

	if err := checkRedirectCycle(dbx, in.From, in.To); err != nil {
		return in, err
	}

	return in, nil
}

func parseRedirectEnabledStrict(raw string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1", "true", "on", "yes":
		return 1, nil
	case "0", "false", "off", "no":
		return 0, nil
	default:
		return 0, errors.New("enabled must be 0 or 1")
	}
}

func normalizeRedirectImportHeader(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "\ufeff")
	return strings.ToLower(raw)
}

func isBlankCSVRow(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func parseRedirectImportCSV(r io.Reader) ([]CreateRedirectInput, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("CSV 文件为空")
		}
		return nil, fmt.Errorf("读取 CSV 表头失败: %w", err)
	}
	if len(header) != len(redirectImportHeaders) {
		return nil, fmt.Errorf("CSV 表头必须为: %s", strings.Join(redirectImportHeaders, ","))
	}
	for i, want := range redirectImportHeaders {
		if normalizeRedirectImportHeader(header[i]) != want {
			return nil, fmt.Errorf("CSV 表头必须为: %s", strings.Join(redirectImportHeaders, ","))
		}
	}

	items := make([]CreateRedirectInput, 0)
	rowNum := 1
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("读取 CSV 第 %d 行失败: %w", rowNum+1, err)
		}
		rowNum++
		if isBlankCSVRow(record) {
			continue
		}
		if len(record) != len(redirectImportHeaders) {
			return nil, fmt.Errorf("CSV 第 %d 行字段数量不正确，期望 %d 列，实际 %d 列", rowNum, len(redirectImportHeaders), len(record))
		}

		status, err := parseRedirectStatusStrict(record[2])
		if err != nil {
			return nil, fmt.Errorf("CSV 第 %d 行 status 无效，只支持 301 或 302", rowNum)
		}
		enabled, err := parseRedirectEnabledStrict(record[3])
		if err != nil {
			return nil, fmt.Errorf("CSV 第 %d 行 enabled 无效，只支持 0 或 1", rowNum)
		}

		items = append(items, CreateRedirectInput{
			From:    strings.TrimSpace(record[0]),
			To:      strings.TrimSpace(record[1]),
			Status:  status,
			Enabled: enabled,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("CSV 文件没有可导入的数据行")
	}

	return items, nil
}

func nextRedirectTarget(dbx *db.DB, pending map[string]string, path string) (string, bool, error) {
	if target, ok := pending[path]; ok {
		return target, true, nil
	}

	redirect, err := db.GetRedirectByFrom(dbx, path)
	if err != nil {
		if db.IsErrNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return redirect.To, true, nil
}

func checkRedirectCycleWithPending(dbx *db.DB, pending map[string]string, fromPath, toPath string) error {
	visited := make(map[string]bool)
	current := toPath
	const maxDepth = 100

	for i := 0; i < maxDepth; i++ {
		if current == fromPath {
			return errors.New("detected redirect cycle: adding this redirect would create a loop")
		}
		if visited[current] {
			return errors.New("detected redirect cycle in existing redirects")
		}
		visited[current] = true

		next, ok, err := nextRedirectTarget(dbx, pending, current)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		current = next
	}

	return errors.New("detected redirect cycle: exceeded maximum redirect chain length")
}

func ImportRedirectCSVService(dbx *db.DB, r io.Reader) (int, error) {
	items, err := parseRedirectImportCSV(r)
	if err != nil {
		return 0, err
	}

	normalized := make([]CreateRedirectInput, 0, len(items))
	pending := make(map[string]string, len(items))
	seenLines := make(map[string]int, len(items))
	for i, item := range items {
		lineNum := i + 2
		validated, err := validateRedirectCreateInput(dbx, item)
		if err != nil {
			return 0, fmt.Errorf("CSV 第 %d 行校验失败: %w", lineNum, err)
		}

		if firstLine, exists := seenLines[validated.From]; exists {
			return 0, fmt.Errorf("CSV 第 %d 行来源路径重复：%s（与第 %d 行重复）", lineNum, validated.From, firstLine)
		}
		seenLines[validated.From] = lineNum

		pending[validated.From] = validated.To
		normalized = append(normalized, validated)
	}

	for i, item := range normalized {
		if err := checkRedirectCycleWithPending(dbx, pending, item.From, item.To); err != nil {
			return 0, fmt.Errorf("CSV 第 %d 行校验失败: %w", i+2, err)
		}
	}

	redirects := make([]db.Redirect, 0, len(normalized))
	for _, item := range normalized {
		redirects = append(redirects, db.Redirect{
			From:    item.From,
			To:      item.To,
			Status:  item.Status,
			Enabled: item.Enabled,
		})
	}

	if err := db.CreateRedirectBatch(dbx, redirects); err != nil {
		return 0, err
	}
	return len(redirects), nil
}
