package site

import (
	"encoding/base64"
	"testing"

	"swaves/internal/platform/db"
)

func TestUVTrackerDeduplicatesRecentEvents(t *testing.T) {
	dbx := newSiteTestDB(t)
	tracker := NewUVTracker(dbx)

	visitorID := testSiteVisitorID(1)
	tracker.Track(db.UVEntitySite, 0, visitorID)
	tracker.Track(db.UVEntitySite, 0, visitorID)
	tracker.Close()

	rows := readSiteUVRows(t, dbx)
	if len(rows) != 1 {
		t.Fatalf("uv rows = %d, want 1", len(rows))
	}
	if rows[0].firstSeenAt != rows[0].lastSeenAt {
		t.Fatalf("duplicate recent event updated row: first=%d last=%d", rows[0].firstSeenAt, rows[0].lastSeenAt)
	}
}

func TestUVTrackerTracksDifferentVisitors(t *testing.T) {
	dbx := newSiteTestDB(t)
	tracker := NewUVTracker(dbx)

	tracker.Track(db.UVEntitySite, 0, testSiteVisitorID(1))
	tracker.Track(db.UVEntitySite, 0, testSiteVisitorID(2))
	tracker.Close()

	rows := readSiteUVRows(t, dbx)
	if len(rows) != 2 {
		t.Fatalf("uv rows = %d, want 2", len(rows))
	}
}

type siteUVRow struct {
	firstSeenAt int64
	lastSeenAt  int64
}

func readSiteUVRows(t *testing.T, dbx *db.DB) []siteUVRow {
	t.Helper()

	rows, err := dbx.Query(
		`SELECT first_seen_at, last_seen_at FROM ` + string(db.TableUniqueVisitors) + ` ORDER BY first_seen_at`,
	)
	if err != nil {
		t.Fatalf("query uv rows failed: %v", err)
	}
	defer rows.Close()

	var out []siteUVRow
	for rows.Next() {
		var row siteUVRow
		if err := rows.Scan(&row.firstSeenAt, &row.lastSeenAt); err != nil {
			t.Fatalf("scan uv row failed: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("uv row iteration failed: %v", err)
	}
	return out
}

func testSiteVisitorID(seed byte) string {
	raw := make([]byte, db.UVVisitorIDBytes)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
