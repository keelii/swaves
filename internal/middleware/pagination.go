// middleware/pagination.go
package middleware

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type Pagination struct {
	Page     int
	PageSize int
	Num      int
	Total    int
}

const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 100
)

// PaginationMiddleware 将 page/pageSize 封装成 Pagination 放入 c.Locals
func PaginationMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := Pagination{
			Page:     DefaultPage,
			PageSize: DefaultPageSize,
		}

		if pageStr := c.Query("page"); pageStr != "" {
			if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
				p.Page = v
			}
		}

		if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
			if v, err := strconv.Atoi(pageSizeStr); err == nil && v > 0 && v <= MaxPageSize {
				p.PageSize = v
			}
		}

		c.Locals("pagination", p)

		return c.Next()
	}
}

// 从 c.Locals 获取 Pagination
func GetPagination(c *fiber.Ctx) Pagination {
	if v := c.Locals("pagination"); v != nil {
		if p, ok := v.(Pagination); ok {
			return p
		}
	}
	return Pagination{Page: DefaultPage, PageSize: DefaultPageSize}
}
