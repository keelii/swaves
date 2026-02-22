package admin

import (
	"strings"
	"testing"

	"swaves/internal/db"
)

func TestFormatCategorySelectLabel(t *testing.T) {
	tests := []struct {
		name  string
		title string
		depth int
		want  string
	}{
		{
			name:  "root depth keeps original title",
			title: "Tech",
			depth: 0,
			want:  "Tech",
		},
		{
			name:  "first level uses branch prefix",
			title: "Go",
			depth: 1,
			want:  "　Go",
		},
		{
			name:  "nested depth keeps full width indent",
			title: "Fiber",
			depth: 3,
			want:  strings.Repeat("　", 3) + "Fiber",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCategorySelectLabel(tt.title, tt.depth)
			if got != tt.want {
				t.Fatalf("unexpected label: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCategorySelectOptionsHierarchy(t *testing.T) {
	categories := []db.Category{
		{ID: 1, ParentID: 0, Name: "Backend"},
		{ID: 2, ParentID: 1, Name: "Go"},
		{ID: 3, ParentID: 2, Name: "Fiber"},
		{ID: 4, ParentID: 1, Name: "Rust"},
		{ID: 5, ParentID: 0, Name: "Frontend"},
	}

	options := BuildCategorySelectOptions(categories)

	wantOrder := []int64{1, 2, 3, 4, 5}
	if len(options) != len(wantOrder) {
		t.Fatalf("unexpected option count: got %d want %d", len(options), len(wantOrder))
	}

	for i, wantID := range wantOrder {
		if options[i].ID != wantID {
			t.Fatalf("unexpected option order at index %d: got %d want %d", i, options[i].ID, wantID)
		}
	}

	wantLabels := map[int64]string{
		1: "Backend",
		2: strings.Repeat("　", 1) + "Go",
		3: strings.Repeat("　", 2) + "Fiber",
		4: strings.Repeat("　", 1) + "Rust",
		5: "Frontend",
	}
	wantDepth := map[int64]int{
		1: 0,
		2: 1,
		3: 2,
		4: 1,
		5: 0,
	}

	for _, option := range options {
		if option.DisplayName != wantLabels[option.ID] {
			t.Fatalf("unexpected display label for id=%d: got %q want %q", option.ID, option.DisplayName, wantLabels[option.ID])
		}
		if option.Depth != wantDepth[option.ID] {
			t.Fatalf("unexpected depth for id=%d: got %d want %d", option.ID, option.Depth, wantDepth[option.ID])
		}
	}
}

func TestBuildCategorySelectOptionsHandlesMissingParentAndCycle(t *testing.T) {
	categories := []db.Category{
		{ID: 10, ParentID: 99, Name: "Orphan"},
		{ID: 11, ParentID: 10, Name: "OrphanChild"},
		{ID: 20, ParentID: 21, Name: "CycleA"},
		{ID: 21, ParentID: 20, Name: "CycleB"},
	}

	options := BuildCategorySelectOptions(categories)
	if len(options) != len(categories) {
		t.Fatalf("unexpected option count: got %d want %d", len(options), len(categories))
	}

	byID := make(map[int64]CategorySelectOption, len(options))
	for _, option := range options {
		byID[option.ID] = option
	}

	if byID[10].Depth != 0 {
		t.Fatalf("orphan category should be treated as root, got depth=%d", byID[10].Depth)
	}
	if byID[11].Depth != 1 {
		t.Fatalf("orphan child depth mismatch: got %d want 1", byID[11].Depth)
	}
	if byID[20].Depth != 0 {
		t.Fatalf("cycle fallback should still include category 20 as root-like option, got depth=%d", byID[20].Depth)
	}
	if byID[21].Depth != 1 {
		t.Fatalf("cycle fallback should still include category 21 once, got depth=%d", byID[21].Depth)
	}
}
