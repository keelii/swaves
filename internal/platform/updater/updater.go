package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/shared/semverutil"
	"time"
)

const (
	GitHubRepositoryURL     = "https://github.com/keelii/swaves"
	gitHubRepositoryAPIURL  = "https://api.github.com/repos/keelii/swaves"
	defaultLatestReleaseURL = gitHubRepositoryAPIURL + "/releases/latest"
)

type ReleaseAsset struct {
	Name        string
	DownloadURL string
}

type ReleaseInfo struct {
	Version     string
	PublishedAt time.Time
	HTMLURL     string
	Assets      []ReleaseAsset
}

type ReleaseTarget struct {
	Archive  ReleaseAsset
	Checksum ReleaseAsset
}

type CheckResult struct {
	CurrentVersion       string
	CurrentVersionStable bool
	LatestVersion        string
	LatestReleaseURL     string
	HasUpgrade           bool
	Target               *ReleaseTarget
	Reason               string
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type githubReleaseResponse struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func DefaultClient() Client {
	return Client{
		BaseURL:    defaultLatestReleaseURL,
		HTTPClient: http.DefaultClient,
	}
}

func CheckLatestRelease(currentVersion string, goos string, goarch string) (CheckResult, error) {
	return DefaultClient().CheckLatestRelease(currentVersion, goos, goarch)
}

func (c Client) CheckLatestRelease(currentVersion string, goos string, goarch string) (CheckResult, error) {
	currentVersion = strings.TrimSpace(currentVersion)
	result := CheckResult{
		CurrentVersion:       currentVersion,
		CurrentVersionStable: semverutil.IsStable(currentVersion),
	}

	release, err := c.LatestStableRelease()
	if err != nil {
		return result, err
	}

	result.LatestVersion = release.Version
	result.LatestReleaseURL = release.HTMLURL

	target := matchReleaseTarget(release, goos, goarch)
	if target == nil {
		result.Reason = fmt.Sprintf("latest release does not provide a supported asset for %s/%s", strings.TrimSpace(goos), strings.TrimSpace(goarch))
		return result, nil
	}
	result.Target = target

	if !result.CurrentVersionStable {
		result.Reason = "current version is not a stable release version"
		return result, nil
	}

	cmp, err := semverutil.Compare(currentVersion, release.Version)
	if err != nil {
		return result, err
	}
	switch {
	case cmp < 0:
		result.HasUpgrade = true
		result.Reason = fmt.Sprintf("upgrade available: %s -> %s", currentVersion, release.Version)
	case cmp == 0:
		result.Reason = "already on latest version"
	default:
		result.Reason = "current version is newer than latest release"
	}

	return result, nil
}

func (c Client) LatestStableRelease() (ReleaseInfo, error) {
	url := strings.TrimSpace(c.BaseURL)
	if url == "" {
		url = defaultLatestReleaseURL
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ReleaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "swaves/"+buildUserAgentVersion())

	resp, err := httpClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("fetch latest release failed: status=%d", resp.StatusCode)
	}

	var payload githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ReleaseInfo{}, err
	}
	if payload.Draft {
		return ReleaseInfo{}, fmt.Errorf("latest release is draft")
	}
	if payload.Prerelease {
		return ReleaseInfo{}, fmt.Errorf("latest release is prerelease")
	}
	if !semverutil.IsStable(payload.TagName) {
		return ReleaseInfo{}, fmt.Errorf("latest release tag is not a stable semver: %q", payload.TagName)
	}

	publishedAt := time.Time{}
	if strings.TrimSpace(payload.PublishedAt) != "" {
		publishedAt, err = time.Parse(time.RFC3339, payload.PublishedAt)
		if err != nil {
			return ReleaseInfo{}, fmt.Errorf("parse published_at failed: %w", err)
		}
	}

	info := ReleaseInfo{
		Version:     strings.TrimSpace(payload.TagName),
		PublishedAt: publishedAt,
		HTMLURL:     strings.TrimSpace(payload.HTMLURL),
		Assets:      make([]ReleaseAsset, 0, len(payload.Assets)),
	}
	for _, asset := range payload.Assets {
		name := strings.TrimSpace(asset.Name)
		downloadURL := strings.TrimSpace(asset.BrowserDownloadURL)
		if name == "" || downloadURL == "" {
			continue
		}
		info.Assets = append(info.Assets, ReleaseAsset{
			Name:        name,
			DownloadURL: downloadURL,
		})
	}
	return info, nil
}

func ReleaseArchiveName(version string, goos string, goarch string) string {
	return fmt.Sprintf("swaves_%s_%s_%s.tar.gz", strings.TrimSpace(version), strings.TrimSpace(goos), strings.TrimSpace(goarch))
}

func ReleaseTagURL(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return GitHubRepositoryURL + "/releases"
	}
	return GitHubRepositoryURL + "/releases/tag/" + version
}

func matchReleaseTarget(release ReleaseInfo, goos string, goarch string) *ReleaseTarget {
	archiveName := ReleaseArchiveName(release.Version, goos, goarch)
	checksumName := archiveName + ".sha256"

	var archive ReleaseAsset
	var checksum ReleaseAsset
	foundArchive := false
	foundChecksum := false

	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			archive = asset
			foundArchive = true
		}
		if asset.Name == checksumName {
			checksum = asset
			foundChecksum = true
		}
	}

	if !foundArchive || !foundChecksum {
		return nil
	}
	return &ReleaseTarget{
		Archive:  archive,
		Checksum: checksum,
	}
}

func buildUserAgentVersion() string {
	version := strings.TrimSpace(buildinfo.Version)
	if version == "" {
		return "dev"
	}
	return version
}
