// middleware/pagination.go
package middleware

import (
	"strconv"
	"swaves/internal/consts"
	"swaves/internal/types"

	"github.com/gofiber/fiber/v2"
)

// PaginationMiddleware 将 page/pageSize 封装成 Pagination 放入 c.Locals
func PaginationMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var pageSize int = consts.DefaultPageSize
		if c.Locals("settings.page_size") != nil {
			size, err := strconv.Atoi(c.Locals("settings.page_size").(string))
			if err == nil {
				pageSize = size
			}
		}

		p := types.Pagination{
			Page:     consts.DefaultPage,
			PageSize: pageSize,
		}

		if pageStr := c.Query("page"); pageStr != "" {
			if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
				p.Page = v
			}
		}

		if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
			if v, err := strconv.Atoi(pageSizeStr); err == nil {
				if v < consts.MinPageSize {
					p.PageSize = consts.MinPageSize
				} else if v > consts.MaxPageSize {
					p.PageSize = consts.MaxPageSize
				} else {
					p.PageSize = v
				}
			}
		}

		c.Locals("pagination", p)

		return c.Next()
	}
}

// 从 c.Locals 获取 Pagination
func GetPagination(c *fiber.Ctx) types.Pagination {
	if v := c.Locals("pagination"); v != nil {
		if p, ok := v.(types.Pagination); ok {
			return p
		}
	}
	return types.Pagination{Page: consts.DefaultPage, PageSize: consts.DefaultPageSize}
}
