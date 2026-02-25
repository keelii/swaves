package admin_app

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

	adminGroup := app.Group("/admin_app")

	adminGroup.Get("/", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_home", fiber.Map{}, "")
	}).Name("admin_app.home")
	adminGroup.Get("/post_edit", func(c fiber.Ctx) error {
		return RenderAdminView(c, "post_edit", fiber.Map{}, "")
	}).Name("admin_app.post_edit")
	adminGroup.Get("/posts_list", func(c fiber.Ctx) error {
		return RenderAdminView(c, "posts_list", fiber.Map{}, "")
	}).Name("admin_app.posts_list")

	uiGroup := adminGroup.Group("/ui")
	uiGroup.Get("/buttons", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/buttons", fiber.Map{}, "")
	}).Name("admin_app.ui.buttons")
	uiGroup.Get("/icons", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/icons", fiber.Map{}, "")
	}).Name("admin_app.ui.icons")
	uiGroup.Get("/forms", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/forms", fiber.Map{}, "")
	}).Name("admin_app.ui.forms")
	uiGroup.Get("/navigation", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/navigation", fiber.Map{}, "")
	}).Name("admin_app.ui.navigation")
	uiGroup.Get("/feedback", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/feedback", fiber.Map{}, "")
	}).Name("admin_app.ui.feedback")
	uiGroup.Get("/data", func(c fiber.Ctx) error {
		return RenderAdminView(c, "admin_app/ui/data", fiber.Map{}, "")
	}).Name("admin_app.ui.data")
}
