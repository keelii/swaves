package admin

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

func RequireLogin(store *SessionStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		fmt.Println("RequireLogin", c.Request().URI())
		sess, err := store.Get(c)
		if err != nil {
			log.Println(err)
			return c.Redirect("/admin/login")
		}

		if sess.Get("admin") != true {
			return c.Redirect("/admin/login")
		}

		return c.Next()
	}
}
