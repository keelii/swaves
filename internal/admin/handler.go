package admin

import (
	"strconv"
	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	DB      *db.DB
	Service *Service
	Store   *SessionStore
}

func NewHandler(db *db.DB, adminService *Service, store *SessionStore) *Handler {
	return &Handler{
		DB:      db,
		Service: adminService,
		Store:   store,
	}
}

func (h *Handler) GetHome(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}
	logined := sess.Get("admin")
	return c.Render("admin_home", fiber.Map{
		"Title":   "Admin Home",
		"IsLogin": logined,
	}, "admin_layout")
}

/* ---------- GET /admin/login ---------- */

func (h *Handler) GetLoginHandler(c *fiber.Ctx) error {
	return c.Render("admin_login", fiber.Map{
		"Title": "Admin Login",
		"Error": "",
	}, "admin_layout")
}

/* ---------- POST /admin/login ---------- */

func (h *Handler) PostLoginHandler(c *fiber.Ctx) error {
	password := c.FormValue("password")
	if password == "" {
		return c.Render("admin_login", fiber.Map{
			"Error": "password is empty",
		}, "admin_layout")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		return c.Render("admin_login", fiber.Map{
			"Title": "Admin Login",
			"Error": "Invalid password",
		}, "admin_layout")
	}

	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}

	sess.Set("admin", true)

	if err := sess.Save(); err != nil {
		return err
	}

	return c.Redirect("/admin")
}

/* ---------- POST /admin/logout ---------- */

func (h *Handler) GetLogoutHandler(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err == nil {
		sess.Destroy()
	}

	return c.Redirect("/admin/login")
}

// Posts
func (h *Handler) GetPostListHandler(c *fiber.Ctx) error {
	posts, err := ListPosts(h.DB)
	if err != nil {
		return err
	}

	return c.Render("posts_index", fiber.Map{
		"Posts": posts,
	}, "admin_layout")
}
func (h *Handler) GetPostNewHandler(c *fiber.Ctx) error {
	return c.Render("posts_new", nil, "admin_layout")
}

func (h *Handler) PostCreatePostHandler(c *fiber.Ctx) error {
	in := CreatePostInput{
		Title:   c.FormValue("title"),
		Slug:    c.FormValue("slug"),
		Content: c.FormValue("content"),
		Status:  c.FormValue("status"),
	}

	if err := CreatePostService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) GetPostEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	post, err := GetPostForEdit(h.DB, id)
	if err != nil {
		return err
	}

	return c.Render("posts_edit", fiber.Map{
		"Post": post,
	}, "admin_layout")
}

func (h *Handler) PostUpdatePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	in := UpdatePostInput{
		Title:   c.FormValue("title"),
		Content: c.FormValue("content"),
		Status:  c.FormValue("status"),
	}

	if err := UpdatePostService(h.DB, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) PostDeletePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeletePostService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}
