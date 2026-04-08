package store

import "testing"

func TestGetRedirectReturnsFalseWhenRedirectMapIsEmpty(t *testing.T) {
	storeRedirectMap(map[string]RedirectRule{})

	if _, ok := GetRedirect("/legacy-path"); ok {
		t.Fatalf("expected empty redirect map lookup to miss")
	}
}

func TestIsRedirectEmptyTracksStoredMap(t *testing.T) {
	storeRedirectMap(map[string]RedirectRule{})
	if !IsRedirectEmpty() {
		t.Fatalf("expected redirect map to be empty")
	}

	storeRedirectMap(map[string]RedirectRule{
		"/legacy-path": {To: "/new-path", Status: 301},
	})
	if IsRedirectEmpty() {
		t.Fatalf("expected redirect map to be non-empty")
	}
}

func TestGetRedirectMatchesTrailingSlashLookup(t *testing.T) {
	storeRedirectMap(map[string]RedirectRule{
		"/2018/08/12/fuzzy-finder-full-guide": {
			To:     "/fuzzy-finder-full-guide",
			Status: 301,
		},
	})

	redirect, ok := GetRedirect("/2018/08/12/fuzzy-finder-full-guide/")
	if !ok {
		t.Fatalf("expected trailing slash lookup to match canonical redirect path")
	}
	if redirect.To != "/fuzzy-finder-full-guide" {
		t.Fatalf("unexpected redirect target: got=%q", redirect.To)
	}
	if redirect.Status != 301 {
		t.Fatalf("unexpected redirect status: got=%d", redirect.Status)
	}
}
