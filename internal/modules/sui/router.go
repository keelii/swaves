package sui

import (
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

func RegisterRouter(
	app *fiber.App,
	_ *store.GlobalStore,
) {
	suiGroup := app.Group("/sui")

	suiGroup.Get("/", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/dash_home.html", fiber.Map{}, "")
	}).Name("sui.home")
	suiGroup.Get("/post_edit", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/post_edit.html", fiber.Map{}, "")
	}).Name("sui.post_edit")
	suiGroup.Get("/posts_list", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/posts_list.html", fiber.Map{}, "")
	}).Name("sui.posts_list")

	uiGroup := suiGroup.Group("/ui")
	uiGroup.Get("/buttons", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/buttons.html", fiber.Map{}, "")
	}).Name("sui.ui.buttons")
	uiGroup.Get("/icons", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/icons.html", fiber.Map{}, "")
	}).Name("sui.ui.icons")
	uiGroup.Get("/forms", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/forms.html", fiber.Map{}, "")
	}).Name("sui.ui.forms")
	uiGroup.Get("/navigation", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/navigation.html", fiber.Map{}, "")
	}).Name("sui.ui.navigation")
	uiGroup.Get("/feedback", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/feedback.html", fiber.Map{}, "")
	}).Name("sui.ui.feedback")
	uiGroup.Get("/data", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/data.html", fiber.Map{}, "")
	}).Name("sui.ui.data")
}
