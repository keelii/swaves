package redirect_rule

import (
	"fmt"
	"sort"
	"strings"
	"swaves/internal/shared/pathutil"
)

type segmentKind int

const (
	segmentLiteral segmentKind = iota
	segmentWildcard
	segmentParam
)

type segment struct {
	kind  segmentKind
	value string
}

type Rule struct {
	From        string
	To          string
	fromParts   []segment
	toParts     []segment
	literalCnt  int
	paramCnt    int
	wildcardCnt int
}

func HasPattern(path string) bool {
	path = strings.TrimSpace(path)
	return strings.Contains(path, "*") || strings.Contains(path, "{")
}

func Compile(fromPath string, toPath string) (Rule, error) {
	fromPath = normalizePath(fromPath)
	toPath = normalizePath(toPath)
	if fromPath == "" || toPath == "" {
		return Rule{}, fmt.Errorf("from and to required")
	}

	fromParts, literalCnt, paramCnt, wildcardCnt, names, err := parseFromSegments(fromPath)
	if err != nil {
		return Rule{}, err
	}
	toParts, err := parseToSegments(toPath, names)
	if err != nil {
		return Rule{}, err
	}

	return Rule{
		From:        fromPath,
		To:          toPath,
		fromParts:   fromParts,
		toParts:     toParts,
		literalCnt:  literalCnt,
		paramCnt:    paramCnt,
		wildcardCnt: wildcardCnt,
	}, nil
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return pathutil.JoinAbsolute(path)
}

func parseFromSegments(path string) ([]segment, int, int, int, map[string]struct{}, error) {
	rawParts := splitSegments(path)
	parts := make([]segment, 0, len(rawParts))
	names := make(map[string]struct{}, len(rawParts))
	literalCnt := 0
	paramCnt := 0
	wildcardCnt := 0

	for _, raw := range rawParts {
		part, err := parsePathSegment(raw, true)
		if err != nil {
			return nil, 0, 0, 0, nil, err
		}
		if part.kind == segmentParam {
			if _, exists := names[part.value]; exists {
				return nil, 0, 0, 0, nil, fmt.Errorf("duplicate redirect variable: %s", part.value)
			}
			names[part.value] = struct{}{}
			paramCnt++
		} else if part.kind == segmentWildcard {
			wildcardCnt++
		} else {
			literalCnt++
		}
		parts = append(parts, part)
	}
	return parts, literalCnt, paramCnt, wildcardCnt, names, nil
}

func parseToSegments(path string, names map[string]struct{}) ([]segment, error) {
	rawParts := splitSegments(path)
	parts := make([]segment, 0, len(rawParts))
	for _, raw := range rawParts {
		part, err := parsePathSegment(raw, false)
		if err != nil {
			return nil, err
		}
		if part.kind == segmentWildcard {
			return nil, fmt.Errorf("redirect target does not support wildcard segment: %s", raw)
		}
		if part.kind == segmentParam {
			if _, ok := names[part.value]; !ok {
				return nil, fmt.Errorf("redirect target variable is undefined: %s", part.value)
			}
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func parsePathSegment(raw string, allowWildcard bool) (segment, error) {
	if raw == "*" {
		if !allowWildcard {
			return segment{}, fmt.Errorf("wildcard is only allowed in redirect source path")
		}
		return segment{kind: segmentWildcard}, nil
	}
	if strings.HasPrefix(raw, "{") || strings.HasSuffix(raw, "}") {
		name, ok := parseParamName(raw)
		if !ok {
			return segment{}, fmt.Errorf("invalid redirect variable segment: %s", raw)
		}
		return segment{kind: segmentParam, value: name}, nil
	}
	if strings.Contains(raw, "{") || strings.Contains(raw, "}") || strings.Contains(raw, "*") {
		return segment{}, fmt.Errorf("unsupported redirect path segment: %s", raw)
	}
	return segment{kind: segmentLiteral, value: raw}, nil
}

func parseParamName(raw string) (string, bool) {
	if len(raw) < 3 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return "", false
	}
	name := raw[1 : len(raw)-1]
	if name == "" {
		return "", false
	}
	for i, ch := range name {
		if i == 0 {
			if !isAlpha(ch) && ch != '_' {
				return "", false
			}
			continue
		}
		if !isAlpha(ch) && !isDigit(ch) && ch != '_' {
			return "", false
		}
	}
	return name, true
}

func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func splitSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func (r Rule) IsPattern() bool {
	return HasPattern(r.From)
}

func (r Rule) Match(path string) (string, bool) {
	path = normalizePath(path)
	if path == "" {
		return "", false
	}
	pathParts := splitSegments(path)
	if len(pathParts) != len(r.fromParts) {
		return "", false
	}

	params := make(map[string]string, r.paramCnt)
	for idx, part := range r.fromParts {
		value := pathParts[idx]
		switch part.kind {
		case segmentLiteral:
			if value != part.value {
				return "", false
			}
		case segmentWildcard:
			if value == "" {
				return "", false
			}
		case segmentParam:
			if value == "" {
				return "", false
			}
			params[part.value] = value
		default:
			return "", false
		}
	}

	if len(r.toParts) == 0 {
		return "/", true
	}
	resolved := make([]string, 0, len(r.toParts))
	for _, part := range r.toParts {
		if part.kind == segmentParam {
			value, ok := params[part.value]
			if !ok || value == "" {
				return "", false
			}
			resolved = append(resolved, value)
			continue
		}
		resolved = append(resolved, part.value)
	}
	return pathutil.JoinAbsolute(strings.Join(resolved, "/")), true
}

func (r Rule) SortKey() [4]int {
	return [4]int{r.literalCnt, r.paramCnt, -r.wildcardCnt, len(r.fromParts)}
}

func SortRules(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		left := rules[i].SortKey()
		right := rules[j].SortKey()
		for idx := 0; idx < len(left); idx++ {
			if left[idx] == right[idx] {
				continue
			}
			return left[idx] > right[idx]
		}
		return rules[i].From < rules[j].From
	})
}
