package semverutil

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease []Identifier
	Build      string
}

type Identifier struct {
	Raw     string
	Numeric bool
	Number  int
}

func Parse(raw string) (Version, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Version{}, fmt.Errorf("version is required")
	}
	if !strings.HasPrefix(text, "v") {
		return Version{}, fmt.Errorf("version must start with v: %q", raw)
	}

	text = strings.TrimPrefix(text, "v")
	build := ""
	if idx := strings.IndexByte(text, '+'); idx >= 0 {
		build = text[idx+1:]
		text = text[:idx]
		if err := validateDotIdentifiers(build, false); err != nil {
			return Version{}, fmt.Errorf("invalid build metadata: %w", err)
		}
	}

	preRelease := ""
	if idx := strings.IndexByte(text, '-'); idx >= 0 {
		preRelease = text[idx+1:]
		text = text[:idx]
	}

	core := strings.Split(text, ".")
	if len(core) != 3 {
		return Version{}, fmt.Errorf("version core must be MAJOR.MINOR.PATCH")
	}

	major, err := parseCoreNumber(core[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := parseCoreNumber(core[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := parseCoreNumber(core[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %w", err)
	}

	version := Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Build: build,
	}
	if preRelease == "" {
		return version, nil
	}

	items, err := parseIdentifiers(preRelease)
	if err != nil {
		return Version{}, fmt.Errorf("invalid prerelease: %w", err)
	}
	version.PreRelease = items
	return version, nil
}

func IsValid(raw string) bool {
	_, err := Parse(raw)
	return err == nil
}

func IsStable(raw string) bool {
	version, err := Parse(raw)
	if err != nil {
		return false
	}
	return len(version.PreRelease) == 0
}

func Compare(a string, b string) (int, error) {
	left, err := Parse(a)
	if err != nil {
		return 0, err
	}
	right, err := Parse(b)
	if err != nil {
		return 0, err
	}
	return compareVersion(left, right), nil
}

func compareVersion(a Version, b Version) int {
	if a.Major != b.Major {
		return compareInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return compareInt(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return compareInt(a.Patch, b.Patch)
	}
	return comparePreRelease(a.PreRelease, b.PreRelease)
}

func compareInt(a int, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePreRelease(a []Identifier, b []Identifier) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}

	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i].Numeric && b[i].Numeric {
			if a[i].Number != b[i].Number {
				return compareInt(a[i].Number, b[i].Number)
			}
			continue
		}
		if a[i].Numeric != b[i].Numeric {
			if a[i].Numeric {
				return -1
			}
			return 1
		}
		if a[i].Raw != b[i].Raw {
			if a[i].Raw < b[i].Raw {
				return -1
			}
			return 1
		}
	}
	return compareInt(len(a), len(b))
}

func parseCoreNumber(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty numeric identifier")
	}
	if len(raw) > 1 && raw[0] == '0' {
		return 0, fmt.Errorf("numeric identifier must not contain leading zeroes: %q", raw)
	}
	for _, r := range raw {
		if !unicode.IsDigit(r) {
			return 0, fmt.Errorf("numeric identifier must contain only digits: %q", raw)
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func parseIdentifiers(raw string) ([]Identifier, error) {
	if err := validateDotIdentifiers(raw, true); err != nil {
		return nil, err
	}

	parts := strings.Split(raw, ".")
	items := make([]Identifier, 0, len(parts))
	for _, part := range parts {
		numeric := isNumeric(part)
		item := Identifier{
			Raw:     part,
			Numeric: numeric,
		}
		if numeric {
			value, err := parseCoreNumber(part)
			if err != nil {
				return nil, err
			}
			item.Number = value
		}
		items = append(items, item)
	}
	return items, nil
}

func validateDotIdentifiers(raw string, validateNumericLeadingZero bool) error {
	if raw == "" {
		return fmt.Errorf("identifier is required")
	}
	parts := strings.Split(raw, ".")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("identifier segment is required")
		}
		for _, r := range part {
			if unicode.IsDigit(r) || unicode.IsLetter(r) || r == '-' {
				continue
			}
			return fmt.Errorf("identifier contains invalid character %q", r)
		}
		if validateNumericLeadingZero && isNumeric(part) && len(part) > 1 && part[0] == '0' {
			return fmt.Errorf("numeric identifier must not contain leading zeroes: %q", part)
		}
	}
	return nil
}

func isNumeric(raw string) bool {
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
