package webutil

import "github.com/gofiber/fiber/v3"

func RedirectTo(c fiber.Ctx, location string, status ...int) error {
	r := c.Redirect()
	if len(status) > 0 {
		r.Status(status[0])
	}
	return r.To(location)
}
