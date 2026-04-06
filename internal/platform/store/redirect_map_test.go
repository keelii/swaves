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
