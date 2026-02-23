// middleware/pagination.go
package middleware

import (
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

// PaginationMiddleware 将 page/pageSize 封装成 Pagination 放入 c.Locals
func PaginationMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		pageSize := consts.DefaultPageSize
		rawPageSize := strings.TrimSpace(fiber.Locals[string](c, "settings.page_size"))
		if rawPageSize != "" {
			size, err := strconv.Atoi(rawPageSize)
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

		fiber.Locals(c, "pagination", p)

		return c.Next()
	}
}

// 从 c.Locals 获取 Pagination
func GetPagination(c fiber.Ctx) types.Pagination {
	p := fiber.Locals[types.Pagination](c, "pagination")
	if p.Page > 0 && p.PageSize > 0 {
		return p
	}
	return types.Pagination{Page: consts.DefaultPage, PageSize: consts.DefaultPageSize}
}
