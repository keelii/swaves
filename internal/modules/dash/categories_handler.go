package dash

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/helper"

	"github.com/gofiber/fiber/v3"
)

func sumCategoryPostCountsWithDescendants(allCategories []db.Category, directCounts map[int64]int, targetIDs []int64) map[int64]int {
	result := make(map[int64]int, len(targetIDs))
	if len(targetIDs) == 0 {
		return result
	}

	childrenByParent := make(map[int64][]int64, len(allCategories))
	for _, category := range allCategories {
		childrenByParent[category.ParentID] = append(childrenByParent[category.ParentID], category.ID)
	}

	memo := make(map[int64]int, len(allCategories))
	visiting := make(map[int64]bool, len(allCategories))
	var countCategoryTree func(id int64) int
	countCategoryTree = func(id int64) int {
		if cached, ok := memo[id]; ok {
			return cached
		}
		if visiting[id] {
			return directCounts[id]
		}

		visiting[id] = true
		total := directCounts[id]
		for _, childID := range childrenByParent[id] {
			total += countCategoryTree(childID)
		}
		visiting[id] = false
		memo[id] = total
		return total
	}

	for _, id := range targetIDs {
		result[id] = countCategoryTree(id)
	}
	return result
}

func collectCategorySelfAndDescendantIDs(allCategories []db.Category, rootID int64) []int64 {
	if rootID <= 0 {
		return []int64{}
	}

	childrenByParent := make(map[int64][]int64, len(allCategories))
	exists := false
	for _, category := range allCategories {
		childrenByParent[category.ParentID] = append(childrenByParent[category.ParentID], category.ID)
		if category.ID == rootID {
			exists = true
		}
	}
	if !exists {
		return []int64{}
	}

	result := make([]int64, 0, len(allCategories))
	stack := []int64{rootID}
	visited := make(map[int64]bool, len(allCategories))
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if visited[id] {
			continue
		}
		visited[id] = true
		result = append(result, id)
		children := childrenByParent[id]
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return result
}

func countCategoryPostsWithDescendants(dbx *db.DB, allCategories []db.Category, targetIDs []int64) (map[int64]int, error) {
	if len(targetIDs) == 0 {
		return map[int64]int{}, nil
	}

	allCategoryIDs := make([]int64, 0, len(allCategories))
	for _, category := range allCategories {
		allCategoryIDs = append(allCategoryIDs, category.ID)
	}

	directCounts, err := db.CountPostsByCategories(dbx, allCategoryIDs)
	if err != nil {
		return nil, err
	}

	return sumCategoryPostCountsWithDescendants(allCategories, directCounts, targetIDs), nil
}

// Categories
func (h *Handler) GetCategoryListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	categories, err := ListCategoriesService(h.Model, &pager)
	if err != nil {
		return err
	}

	allCategories, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

	// 创建分类ID到名称的映射，方便显示父分类名称
	categoryMap := make(map[int64]string)
	for _, cat := range allCategories {
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
	postCounts, err := countCategoryPostsWithDescendants(h.Model, allCategories, categoryIDs)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/categories_index.html", fiber.Map{
		"Title":      "Categories",
		"Categories": categories,
		"ParentMap":  parentMap,
		"Pager":      pager,
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

	return RenderDashView(c, "dash/categories_tree.html", fiber.Map{
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

	return RenderDashView(c, "dash/categories_new.html", fiber.Map{
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
		return RenderDashView(c, "dash/categories_new.html", fiber.Map{
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
		return RenderDashView(c, "dash/categories_new.html", fiber.Map{
			"Title":           "New Category",
			"Error":           err.Error(),
			"Categories":      []db.Category{},
			"CategoryOptions": []CategorySelectOption{},
			"ParentID":        parentID,
		}, "")
	}

	return h.redirectToDashRoute(c, "dash.categories.list", nil, nil)
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

	return RenderDashView(c, "dash/categories_edit.html", fiber.Map{
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
		return RenderDashView(c, "dash/categories_edit.html", fiber.Map{
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
		return RenderDashView(c, "dash/categories_edit.html", fiber.Map{
			"Title":           "Edit Category",
			"Error":           err.Error(),
			"Category":        category,
			"Categories":      all,
			"CategoryOptions": BuildCategorySelectOptions(all),
		}, "")
	}

	return h.redirectToDashRoute(c, "dash.categories.list", nil, nil)
}

func (h *Handler) PostDeleteCategoryHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteCategoryService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.categories.list", nil, nil)
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

	return h.redirectToDashRoute(c, "dash.categories.tree", nil, nil)
}
