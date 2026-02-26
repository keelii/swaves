package sui

import (
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

func RegisterRouter(
	app *fiber.App,
	gStore *store.GlobalStore,
) {
	//monitorStore := NewMonitorStore()
	//handler := NewHandler(
	//	gStore,
	//	NewService(gStore.Model),
	//	monitorStore,
	//)

	suiGroup := app.Group("/sui")

	suiGroup.Get("/", func(c fiber.Ctx) error {
		return RenderSUIView(c, "admin_home", fiber.Map{}, "")
	}).Name("sui.home")
	suiGroup.Get("/post_edit", func(c fiber.Ctx) error {
		return RenderSUIView(c, "post_edit", fiber.Map{}, "")
	}).Name("sui.post_edit")
	suiGroup.Get("/posts_list", func(c fiber.Ctx) error {
		return RenderSUIView(c, "posts_list", fiber.Map{}, "")
	}).Name("sui.posts_list")

	uiGroup := suiGroup.Group("/ui")
	uiGroup.Get("/buttons", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/buttons", fiber.Map{}, "")
	}).Name("sui.ui.buttons")
	uiGroup.Get("/icons", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/icons", fiber.Map{}, "")
	}).Name("sui.ui.icons")
	uiGroup.Get("/forms", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/forms", fiber.Map{}, "")
	}).Name("sui.ui.forms")
	uiGroup.Get("/navigation", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/navigation", fiber.Map{}, "")
	}).Name("sui.ui.navigation")
	uiGroup.Get("/feedback", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/feedback", fiber.Map{}, "")
	}).Name("sui.ui.feedback")
	uiGroup.Get("/data", func(c fiber.Ctx) error {
		return RenderSUIView(c, "sui/ui/data", fiber.Map{}, "")
	}).Name("sui.ui.data")
}
