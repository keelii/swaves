package tpl

import (
	"github.com/gofiber/fiber/v2"
)

// RenderTemplate 将模板渲染集成到 Fiber 上下文
func RenderTemplate(c *fiber.Ctx, name string, data any) error {
	c.Set("Content-Type", "text/html; charset=utf-8")
	return Render(c.Response().BodyWriter(), name, data)
}
