package updater

import (
	"net/http"
	"testing"
)

func TestCheckLatestReleaseFindsStableUpgrade(t *testing.T) {
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{
				"tag_name":"v1.2.4",
				"html_url":"https://github.com/keelii/swaves/releases/tag/v1.2.4",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz"},
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz.sha256"}
				]
			}`
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	result, err := client.CheckLatestRelease("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("CheckLatestRelease failed: %v", err)
	}
	if !result.HasUpgrade {
		t.Fatal("expected upgrade to be available")
	}
	if result.Target == nil {
		t.Fatal("expected matching release target")
	}
	if result.Target.Archive.Name != "swaves_v1.2.4_linux_amd64.tar.gz" {
		t.Fatalf("unexpected archive name: %q", result.Target.Archive.Name)
	}
}

func TestCheckLatestReleaseSkipsNonReleaseCurrentVersion(t *testing.T) {
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{
				"tag_name":"v1.2.4",
				"html_url":"https://github.com/keelii/swaves/releases/tag/v1.2.4",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz"},
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz.sha256"}
				]
			}`
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	result, err := client.CheckLatestRelease("dev", "linux", "amd64")
	if err != nil {
		t.Fatalf("CheckLatestRelease failed: %v", err)
	}
	if result.CurrentVersionStable {
		t.Fatal("expected dev not to be treated as stable")
	}
	if result.HasUpgrade {
		t.Fatal("expected no semver upgrade decision for dev build")
	}
	if result.Reason != "current version is not a stable release version" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
}

func TestCheckLatestReleaseRejectsPrereleaseTag(t *testing.T) {
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{
				"tag_name":"v1.2.4-rc.1",
				"html_url":"https://github.com/keelii/swaves/releases/tag/v1.2.4-rc.1",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":true,
				"assets":[]
			}`
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	if _, err := client.CheckLatestRelease("v1.2.3", "linux", "amd64"); err == nil {
		t.Fatal("expected prerelease latest release to be rejected")
	}
}
