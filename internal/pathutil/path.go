package pathutil

import "strings"

func splitPathSegments(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func JoinAbsolute(parts ...string) string {
	segments := make([]string, 0, len(parts)*2)
	for _, part := range parts {
		segments = append(segments, splitPathSegments(part)...)
	}
	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/")
}

func JoinRelative(parts ...string) string {
	segments := make([]string, 0, len(parts)*2)
	for _, part := range parts {
		segments = append(segments, splitPathSegments(part)...)
	}
	return strings.Join(segments, "/")
}
