package dash

import (
	"strings"
	"testing"

	"swaves/internal/platform/db"
)

func TestImportRedirectCSVServiceImportsRows(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	csvData := strings.Join([]string{
		"from,to,status,enabled",
		"/legacy-one,/new-one,301,1",
		"/legacy-two,/new-two,302,0",
	}, "\n")

	count, err := ImportRedirectCSVService(dbx, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("import redirects failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("unexpected imported count: got %d want 2", count)
	}

	first, err := db.GetRedirectByFrom(dbx, "/legacy-one")
	if err != nil {
		t.Fatalf("expected first redirect: %v", err)
	}
	if first.To != "/new-one" || first.Status != 301 || first.Enabled != 1 {
		t.Fatalf("unexpected first redirect: %+v", first)
	}

	second, err := db.GetRedirectByFrom(dbx, "/legacy-two")
	if err != nil {
		t.Fatalf("expected second redirect: %v", err)
	}
	if second.To != "/new-two" || second.Status != 302 || second.Enabled != 0 {
		t.Fatalf("unexpected second redirect: %+v", second)
	}
}

func TestImportRedirectCSVServiceRejectsInvalidHeader(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	csvData := strings.Join([]string{
		"source,target,status,enabled",
		"/legacy-one,/new-one,301,1",
	}, "\n")

	_, err := ImportRedirectCSVService(dbx, strings.NewReader(csvData))
	if err == nil {
		t.Fatal("expected invalid header error")
	}
	if !strings.Contains(err.Error(), "CSV 表头必须为") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportRedirectCSVServiceRejectsInvalidFieldCount(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	csvData := strings.Join([]string{
		"from,to,status,enabled",
		"/legacy-one,/new-one,301",
	}, "\n")

	_, err := ImportRedirectCSVService(dbx, strings.NewReader(csvData))
	if err == nil {
		t.Fatal("expected invalid field count error")
	}
	if !strings.Contains(err.Error(), "字段数量不正确") {
		t.Fatalf("unexpected error: %v", err)
	}
}
