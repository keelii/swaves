package dash

import (
	"testing"

	"swaves/internal/platform/db"
)

func TestSumCategoryPostCountsWithDescendants(t *testing.T) {
	categories := []db.Category{
		{ID: 1, ParentID: 0, Name: "Root"},
		{ID: 2, ParentID: 1, Name: "Child"},
		{ID: 3, ParentID: 2, Name: "Grandchild"},
		{ID: 4, ParentID: 0, Name: "Sibling"},
	}
	directCounts := map[int64]int{
		1: 1,
		2: 2,
		3: 3,
		4: 4,
	}

	got := sumCategoryPostCountsWithDescendants(categories, directCounts, []int64{1, 2, 4, 999})

	if got[1] != 6 {
		t.Fatalf("root count = %d, want 6", got[1])
	}
	if got[2] != 5 {
		t.Fatalf("child count = %d, want 5", got[2])
	}
	if got[4] != 4 {
		t.Fatalf("sibling count = %d, want 4", got[4])
	}
	if got[999] != 0 {
		t.Fatalf("missing category count = %d, want 0", got[999])
	}
}

func TestCollectCategorySelfAndDescendantIDs(t *testing.T) {
	categories := []db.Category{
		{ID: 1, ParentID: 0, Name: "Root"},
		{ID: 2, ParentID: 1, Name: "Child A"},
		{ID: 3, ParentID: 1, Name: "Child B"},
		{ID: 4, ParentID: 2, Name: "Grandchild"},
	}

	got := collectCategorySelfAndDescendantIDs(categories, 1)
	want := []int64{1, 2, 4, 3}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}

	missing := collectCategorySelfAndDescendantIDs(categories, 99)
	if len(missing) != 0 {
		t.Fatalf("missing root should return empty slice, got %+v", missing)
	}
}
