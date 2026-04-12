// middleware/pagination.go
package middleware

import (
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

// PaginationMiddleware 将 page/pageSize 封装成 Pagination 放入 c.Locals
func PaginationMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		pageSize := resolveDefaultPageSize(c)

		p := types.Pagination{
			Page:     config.DefaultPage,
			PageSize: pageSize,
		}

		if pageStr := c.Query("page"); pageStr != "" {
			if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
				p.Page = v
			}
		}

		if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
			if v, err := strconv.Atoi(pageSizeStr); err == nil {
				if v < config.MinPageSize {
					p.PageSize = config.MinPageSize
				} else if v > config.MaxPageSize {
					p.PageSize = config.MaxPageSize
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
	return types.Pagination{Page: config.DefaultPage, PageSize: resolveDefaultPageSize(c)}
}

func resolveDefaultPageSize(c fiber.Ctx) int {
	pageSize := config.DefaultPageSize
	settingCode := "page_size"
	if isDashRequest(c) {
		settingCode = "dash_page_size"
	}

	if size, ok := parsePageSizeSetting(fiber.Locals[string](c, "settings."+settingCode)); ok {
		return size
	}
	if settingCode != "page_size" {
		if size, ok := parsePageSizeSetting(fiber.Locals[string](c, "settings.page_size")); ok {
			return size
		}
	}
	return pageSize
}

func parsePageSizeSetting(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size <= 0 {
		return 0, false
	}
	return size, true
}

func isDashRequest(c fiber.Ctx) bool {
	path := strings.TrimSpace(c.Path())
	dashBasePath := strings.TrimSpace(share.GetDashUrl())
	if path == "" || dashBasePath == "" || dashBasePath == "/" {
		return false
	}
	return path == dashBasePath || strings.HasPrefix(path, dashBasePath+"/")
}
