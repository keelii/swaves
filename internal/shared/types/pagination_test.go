package types

import (
	"strconv"
	"testing"
)

func TestPaginationGetPageItems(t *testing.T) {
	pager := Pagination{Page: 50, Num: 100}
	items := pager.GetPageItems()

	got := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case PageItemTypeEllipsis:
			got = append(got, "...")
		case PageItemTypeNumber:
			if item.Current {
				got = append(got, "["+strconv.Itoa(item.Page)+"]")
			} else {
				got = append(got, strconv.Itoa(item.Page))
			}
		}
	}

	want := []string{"1", "...", "48", "49", "[50]", "51", "52", "...", "100"}
	if len(got) != len(want) {
		t.Fatalf("unexpected token count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected token at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestPaginationGetPageItemsAtEdges(t *testing.T) {
	start := Pagination{Page: 1, Num: 100}
	startItems := start.GetPageItems()
	if !containsPage(startItems, 1, true) {
		t.Fatalf("expected current page 1 in start items: %+v", startItems)
	}
	if !containsPage(startItems, 100, false) {
		t.Fatalf("expected page 100 in start items: %+v", startItems)
	}
	if !containsEllipsis(startItems) {
		t.Fatalf("expected ellipsis in start items: %+v", startItems)
	}

	end := Pagination{Page: 100, Num: 100}
	endItems := end.GetPageItems()
	if !containsPage(endItems, 100, true) {
		t.Fatalf("expected current page 100 in end items: %+v", endItems)
	}
	if !containsPage(endItems, 1, false) {
		t.Fatalf("expected page 1 in end items: %+v", endItems)
	}
	if !containsEllipsis(endItems) {
		t.Fatalf("expected ellipsis in end items: %+v", endItems)
	}
}

func TestPaginationGetPageItemsSmallTotal(t *testing.T) {
	pager := Pagination{Page: 3, Num: 5}
	items := pager.GetPageItems()
	if containsEllipsis(items) {
		t.Fatalf("did not expect ellipsis for small total: %+v", items)
	}
	for page := 1; page <= 5; page++ {
		wantCurrent := page == 3
		if !containsPage(items, page, wantCurrent) {
			t.Fatalf("expected page %d current=%v in items: %+v", page, wantCurrent, items)
		}
	}
}

func TestPaginationPrevNext(t *testing.T) {
	pager := Pagination{Page: 1, Num: 10}
	if pager.HasPrev() {
		t.Fatal("page 1 should not have prev")
	}
	if !pager.HasNext() {
		t.Fatal("page 1 should have next")
	}
	if pager.PrevPage() != 1 {
		t.Fatalf("prev of page 1 should be 1, got %d", pager.PrevPage())
	}
	if pager.NextPage() != 2 {
		t.Fatalf("next of page 1 should be 2, got %d", pager.NextPage())
	}

	pager = Pagination{Page: 10, Num: 10}
	if !pager.HasPrev() {
		t.Fatal("last page should have prev")
	}
	if pager.HasNext() {
		t.Fatal("last page should not have next")
	}
	if pager.PrevPage() != 9 {
		t.Fatalf("prev of last page should be 9, got %d", pager.PrevPage())
	}
	if pager.NextPage() != 10 {
		t.Fatalf("next of last page should be 10, got %d", pager.NextPage())
	}
}

func containsEllipsis(items []PageItem) bool {
	for _, item := range items {
		if item.Type == PageItemTypeEllipsis {
			return true
		}
	}
	return false
}

func containsPage(items []PageItem, page int, current bool) bool {
	for _, item := range items {
		if item.Type != PageItemTypeNumber {
			continue
		}
		if item.Page == page && item.Current == current {
			return true
		}
	}
	return false
}
