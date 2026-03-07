package sui

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

func RenderSUIView(c fiber.Ctx, view string, data fiber.Map, layout string) error {
	_ = layout

	if data == nil {
		data = fiber.Map{}
	}

	routeName := ""
	if route := c.Route(); route != nil {
		routeName = strings.TrimSpace(route.Name)
	}
	data["RouteName"] = routeName
	data["Query"] = c.Queries()
	data["IsLogin"] = fiber.Locals[bool](c, "IsLogin")
	data["_csrf_token_value"] = fiber.Locals[string](c, "CsrfToken")

	return c.Render(view, data)
}
