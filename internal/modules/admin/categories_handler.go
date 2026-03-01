package admin

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/shared/helper"

	"github.com/gofiber/fiber/v3"
)

// Categories
func (h *Handler) GetCategoryListHandler(c fiber.Ctx) error {
	categories, err := ListCategoriesService(h.Model)
	if err != nil {
		return err
	}

	// 创建分类ID到名称的映射，方便显示父分类名称
	categoryMap := make(map[int64]string)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat.Name
	}

	// 创建父分类名称映射
	parentMap := make(map[int64]string)
	for _, cat := range categories {
		if cat.ParentID > 0 {
			if parentName, ok := categoryMap[cat.ParentID]; ok {
				parentMap[cat.ID] = parentName
			}
		}
	}

	// 统计每个分类的文章数量
	categoryIDs := make([]int64, len(categories))
	for i, cat := range categories {
		categoryIDs[i] = cat.ID
	}
	postCounts, err := db.CountPostsByCategories(h.Model, categoryIDs)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "dash/categories_index.html", fiber.Map{
		"Title":      "Categories",
		"Categories": categories,
		"ParentMap":  parentMap,
		"PostCounts": postCounts,
	}, "")
}

func (h *Handler) GetCategoryTreeHandler(c fiber.Ctx) error {
	allCategories, tree, err := GetCategoryTree(h.Model)
	if err != nil {
		return err
	}

	//allCategories, err := GetAllCategoriesFlat(h.Model)
	//if err != nil {
	//	return err
	//}

	return RenderAdminView(c, "dash/categories_tree.html", fiber.Map{
		"Title":           "Category Tree",
		"Tree":            tree,
		"Categories":      allCategories,
		"CategoryOptions": BuildCategorySelectOptions(allCategories),
	}, "")
}

func (h *Handler) GetCategoryNewHandler(c fiber.Ctx) error {
	// 获取所有分类用于选择父级
	all, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

	// 从查询参数获取预选的父分类 ID
	parentIDStr := c.Query("parent_id", "")
	var parentID int64
	if parentIDStr != "" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	return RenderAdminView(c, "dash/categories_new.html", fiber.Map{
		"Title":           "New Category",
		"Categories":      all,
		"CategoryOptions": BuildCategorySelectOptions(all),
		"ParentID":        parentID,
	}, "")
}

func (h *Handler) PostCreateCategoryHandler(c fiber.Ctx) error {
	parentIDStr := c.FormValue("parent_id")
	var parentID int64
	if parentIDStr != "" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	sortStr := c.FormValue("sort")
	var sort int64
	if sortStr != "" {
		var err error
		sort, err = strconv.ParseInt(sortStr, 10, 64)
		if err != nil {
			sort = 0
		}
	}

	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "dash/categories_new.html", fiber.Map{
			"Title":           "New Category",
			"Error":           errSlugInvalid("013", slug).Error(),
			"Categories":      all,
			"CategoryOptions": BuildCategorySelectOptions(all),
			"ParentID":        parentID,
		}, "")
	}

	in := CreateCategoryInput{
		ParentID:    parentID,
		Name:        c.FormValue("name"),
		Slug:        slug,
		Description: c.FormValue("description"),
		Sort:        sort,
	}

	if err := CreateCategoryService(h.Model, in); err != nil {
		return RenderAdminView(c, "dash/categories_new.html", fiber.Map{
			"Title":           "New Category",
			"Error":           err.Error(),
			"Categories":      []db.Category{},
			"CategoryOptions": []CategorySelectOption{},
			"ParentID":        parentID,
		}, "")
	}

	return h.redirectToAdminRoute(c, "admin.categories.list", nil, nil)
}

func (h *Handler) GetCategoryEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	category, err := GetCategoryForEdit(h.Model, id)
	if err != nil {
		return err
	}

	// 获取所有分类用于选择父级（排除自己）
	all, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

	// 过滤掉自己和自己的子节点（防止循环）
	var availableCategories []db.Category
	for _, c := range all {
		if c.ID == id {
			continue
		}
		// 检查是否是当前分类的子节点
		isChild := false
		cur := c.ParentID
		for cur != 0 {
			if cur == id {
				isChild = true
				break
			}
			// 找到父节点
			var parent *db.Category
			for _, p := range all {
				if p.ID == cur {
					parent = &p
					break
				}
			}
			if parent == nil {
				break
			}
			cur = parent.ParentID
		}
		if !isChild {
			availableCategories = append(availableCategories, c)
		}
	}

	return RenderAdminView(c, "dash/categories_edit.html", fiber.Map{
		"Title":           "Edit Category",
		"Category":        category,
		"Categories":      availableCategories,
		"CategoryOptions": BuildCategorySelectOptions(availableCategories),
	}, "")
}

func (h *Handler) PostUpdateCategoryHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	parentIDStr := c.FormValue("parent_id")
	var parentID int64
	if parentIDStr != "" && parentIDStr != "0" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	sortStr := c.FormValue("sort")
	var sort int64
	if sortStr != "" {
		var err error
		sort, err = strconv.ParseInt(sortStr, 10, 64)
		if err != nil {
			sort = 0
		}
	}

	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		category, _ := GetCategoryForEdit(h.Model, id)
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "dash/categories_edit.html", fiber.Map{
			"Title":           "Edit Category",
			"Error":           errSlugInvalid("014", slug).Error(),
			"Category":        category,
			"Categories":      all,
			"CategoryOptions": BuildCategorySelectOptions(all),
		}, "")
	}

	in := UpdateCategoryInput{
		ParentID:    parentID,
		Name:        c.FormValue("name"),
		Slug:        slug,
		Description: c.FormValue("description"),
		Sort:        sort,
	}

	if err := UpdateCategoryService(h.Model, id, in); err != nil {
		category, _ := GetCategoryForEdit(h.Model, id)
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "dash/categories_edit.html", fiber.Map{
			"Title":           "Edit Category",
			"Error":           err.Error(),
			"Category":        category,
			"Categories":      all,
			"CategoryOptions": BuildCategorySelectOptions(all),
		}, "")
	}

	return h.redirectToAdminRoute(c, "admin.categories.list", nil, nil)
}

func (h *Handler) PostDeleteCategoryHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteCategoryService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.categories.list", nil, nil)
}

func (h *Handler) PostUpdateCategoryParentHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	parentIDStr := c.FormValue("categories")
	var parentID int64
	if parentIDStr != "" && parentIDStr != "0" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	if err := UpdateCategoryParentService(h.Model, id, parentID); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.categories.tree", nil, nil)
}
