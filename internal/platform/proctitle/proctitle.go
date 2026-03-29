package proctitle

import (
	"strings"

	"github.com/erikdubbelboer/gspt"
)

func Set(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}

	// Best-effort only. If setting process title is unsupported on a platform,
	// gspt will no-op or panic; we keep runtime behavior safe.
	defer func() {
		_ = recover()
	}()

	gspt.SetProcTitle(title)
}
