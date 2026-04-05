package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
	"swaves/internal/shared/semverutil"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = ""
)

func NormalizedVersion() string {
	version := strings.TrimSpace(Version)
	if !semverutil.IsValid(version) {
		return ""
	}
	return version
}

func IsReleaseVersion() bool {
	return semverutil.IsStable(strings.TrimSpace(Version))
}

func Summary() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}

	commit := strings.TrimSpace(Commit)
	if commit == "" {
		commit = "unknown"
	}

	buildTime := strings.TrimSpace(BuildTime)
	if buildTime == "" {
		buildTime = "unknown"
	}

	return fmt.Sprintf(
		"swaves %s\ncommit: %s\nbuilt: %s\nplatform: %s/%s\n",
		version,
		commit,
		buildTime,
		runtime.GOOS,
		runtime.GOARCH,
	)
}
