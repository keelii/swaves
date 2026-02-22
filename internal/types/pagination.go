package types

import "sort"

type Pagination struct {
	Page     int
	PageSize int
	Num      int
	Total    int
}

type PageItemType string

const (
	PageItemTypeNumber   PageItemType = "number"
	PageItemTypeEllipsis PageItemType = "ellipsis"
)

type PageItem struct {
	Type    PageItemType
	Page    int
	Current bool
}

func (p Pagination) GetPageItems() []PageItem {
	return p.BuildPageItems(2, 1)
}

func (p Pagination) GetPageNumbers() []int {
	items := p.GetPageItems()
	numbers := make([]int, 0, len(items))
	for _, item := range items {
		if item.Type == PageItemTypeNumber {
			numbers = append(numbers, item.Page)
		}
	}
	return numbers
}

func (p Pagination) BuildPageItems(window int, edge int) []PageItem {
	if p.Num <= 0 {
		return nil
	}

	if window < 0 {
		window = 0
	}
	if edge < 1 {
		edge = 1
	}

	current := p.Page
	if current < 1 {
		current = 1
	}
	if current > p.Num {
		current = p.Num
	}

	keptPages := map[int]struct{}{}
	addPage := func(page int) {
		if page < 1 || page > p.Num {
			return
		}
		keptPages[page] = struct{}{}
	}
	addRange := func(start int, end int) {
		if start > end {
			return
		}
		for page := start; page <= end; page++ {
			addPage(page)
		}
	}

	addRange(1, edge)
	addRange(p.Num-edge+1, p.Num)
	addRange(current-window, current+window)

	sortedPages := make([]int, 0, len(keptPages))
	for page := range keptPages {
		sortedPages = append(sortedPages, page)
	}
	sort.Ints(sortedPages)
	if len(sortedPages) == 0 {
		return nil
	}

	items := make([]PageItem, 0, len(sortedPages)+2)
	lastPage := 0
	for _, page := range sortedPages {
		if lastPage > 0 && page-lastPage > 1 {
			items = append(items, PageItem{Type: PageItemTypeEllipsis})
		}
		items = append(items, PageItem{
			Type:    PageItemTypeNumber,
			Page:    page,
			Current: page == current,
		})
		lastPage = page
	}
	return items
}

func (p Pagination) HasPrev() bool {
	return p.Num > 0 && p.Page > 1
}

func (p Pagination) HasNext() bool {
	return p.Num > 0 && p.Page < p.Num
}

func (p Pagination) PrevPage() int {
	if !p.HasPrev() {
		return 1
	}
	return p.Page - 1
}

func (p Pagination) NextPage() int {
	if !p.HasNext() {
		if p.Num > 0 {
			return p.Num
		}
		return 1
	}
	return p.Page + 1
}
