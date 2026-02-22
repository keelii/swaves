package types

import (
	"fmt"
	"sort"

	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

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

func (p Pagination) normalized() Pagination {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 10
	}
	if p.Num < 0 {
		p.Num = 0
	}
	if p.Total < 0 {
		p.Total = 0
	}
	if p.Num > 0 && p.Page > p.Num {
		p.Page = p.Num
	}
	return p
}

func (p Pagination) GetAttr(name string) value.Value {
	normalized := p.normalized()
	switch name {
	case "Page":
		return value.FromInt(int64(normalized.Page))
	case "PageSize":
		return value.FromInt(int64(normalized.PageSize))
	case "Num":
		return value.FromInt(int64(normalized.Num))
	case "Total":
		return value.FromInt(int64(normalized.Total))
	default:
		return value.Undefined()
	}
}

func (p Pagination) CallMethod(_ value.State, name string, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	if len(kwargs) > 0 {
		return value.Undefined(), fmt.Errorf("pagination method %q does not support keyword arguments", name)
	}
	if len(args) > 0 {
		return value.Undefined(), fmt.Errorf("pagination method %q does not support positional arguments", name)
	}
	normalized := p.normalized()
	switch name {
	case "GetPageItems":
		return value.FromAny(normalized.GetPageItems()), nil
	case "HasPrev":
		return value.FromBool(normalized.HasPrev()), nil
	case "HasNext":
		return value.FromBool(normalized.HasNext()), nil
	case "PrevPage":
		return value.FromInt(int64(normalized.PrevPage())), nil
	case "NextPage":
		return value.FromInt(int64(normalized.NextPage())), nil
	default:
		return value.Undefined(), value.ErrUnknownMethod
	}
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
