package view

import (
	"bytes"
	"strings"
	"testing"
	"time"

	dash "swaves/internal/modules/dash"
	"swaves/internal/modules/site"
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func mustLoadRegressionView(t *testing.T) *FiberView {
	t.Helper()

	view, _ := NewViewEngine(testTemplateRoot(), false)
	fiberView, ok := view.(*FiberView)
	if !ok {
		t.Fatal("expected FiberView from NewViewEngine")
	}
	if err := fiberView.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}
	return fiberView
}

func mustLoadRegressionViewWithResolver(t *testing.T) (*FiberView, func(app *fiber.App)) {
	t.Helper()

	view, initURLResolver := NewViewEngine(testTemplateRoot(), false)
	fiberView, ok := view.(*FiberView)
	if !ok {
		t.Fatal("expected FiberView from NewViewEngine")
	}
	if err := fiberView.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}
	return fiberView, initURLResolver
}

func mustRenderRegressionTemplate(t *testing.T, view *FiberView, template string, context map[string]any) string {
	t.Helper()

	var out bytes.Buffer
	if err := view.Render(&out, template, context); err != nil {
		t.Fatalf("render %s failed: %v", template, err)
	}
	return out.String()
}

func withRegressionSettings(t *testing.T, settings map[string]string) {
	t.Helper()

	previous, _ := store.Settings.Load().(map[string]string)
	restore := make(map[string]string, len(previous))
	for key, value := range previous {
		restore[key] = value
	}
	next := make(map[string]string, len(settings))
	for key, value := range settings {
		next[key] = value
	}
	store.Settings.Store(next)
	t.Cleanup(func() {
		store.Settings.Store(restore)
	})
}

func TestRenderDashCategoriesIndexWithMissingCounts(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/categories_index.html", map[string]any{
		"Categories": []db.Category{
			{
				ID:          1,
				ParentID:    7,
				Name:        "Category A",
				Slug:        "category-a",
				Description: "desc",
				Sort:        1,
				CreatedAt:   1,
				UpdatedAt:   1,
			},
		},
		"ParentMap":  map[int64]string{},
		"PostCounts": map[int64]int{},
		"Pager":      types.Pagination{Page: 1, Num: 1, Total: 1, PageSize: 10},
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderDashPostsIndexWithoutFilterNames(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/posts_index.html", map[string]any{
		"Posts":                   []db.PostWithRelation{},
		"Pager":                   types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 10},
		"Kind":                    db.PostKindPost,
		"KindQuery":               "0",
		"CountPost":               0,
		"CountPage":               0,
		"SearchQuery":             "",
		"SearchQueryEscaped":      "",
		"FilterTagIDStr":          "",
		"FilterCategoryIDStr":     "",
		"FilterTagName":           "",
		"FilterCategoryName":      "",
		"FilterTagRemoveURL":      "",
		"FilterCategoryRemoveURL": "",
		"PostUVMap":               map[int64]int{},
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, "多选") {
		t.Fatal("expected posts index to render multiselect toggle")
	}
}

func TestRenderDashRecordsIndexDoesNotRenderMultiselect(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/records_index.html", map[string]any{
		"RouteName": "dash.records.list",
	})
	if strings.Contains(rendered, "多选") {
		t.Fatal("expected records index to hide multiselect toggle")
	}
}

func TestRenderDashTaskRunsIndexDoesNotRenderMultiselect(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/task_runs_index.html", map[string]any{
		"RouteName": "dash.tasks.runs",
		"Task":      db.Task{Name: "demo", Code: "demo"},
		"Runs":      []db.TaskRun{},
	})
	if strings.Contains(rendered, "多选") {
		t.Fatal("expected task runs index to hide multiselect toggle")
	}
}

func TestRenderSitePostWithEmbeddedDisplayPost(t *testing.T) {
	view := mustLoadRegressionView(t)

	post := site.DisplayPostWithRelation{
		DisplayPost: site.DisplayPost{
			Post: db.Post{
				ID:             1,
				Kind:           0,
				Title:          "hello",
				PublishedAt:    1,
				CommentEnabled: 1,
			},
			HTML: "<p>hello</p>",
		},
	}

	rendered := mustRenderRegressionTemplate(t, view, "site/post.html", map[string]any{
		"Post":                   post,
		"ReadUV":                 0,
		"LikeCount":              0,
		"Liked":                  false,
		"Comments":               []site.DisplayComment{},
		"CommentCount":           0,
		"CommentFeedback":        "",
		"CommentForm":            map[string]any{},
		"CommentCaptchaRequired": false,
		"CommentCaptcha":         map[string]any{},
		"UrlPath":                "/hello",
		"IsLogin":                false,
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("expected rendered post title/content")
	}
	if !strings.Contains(rendered, "<p>hello</p>") {
		t.Fatalf("expected rendered html content")
	}
	if !strings.Contains(rendered, "/static/katex/katex.min.css") {
		t.Fatalf("expected math assets on site post detail")
	}
}

func TestRenderSitePostWithCommentTree(t *testing.T) {
	view := mustLoadRegressionView(t)

	post := site.DisplayPostWithRelation{
		DisplayPost: site.DisplayPost{
			Post: db.Post{
				ID:             2,
				Kind:           db.PostKindPost,
				Title:          "with-comments",
				PublishedAt:    1,
				CommentEnabled: 1,
			},
			HTML: "<p>body</p>",
		},
	}

	comments := []*site.DisplayComment{
		{
			Comment: db.Comment{
				ID:          101,
				PostID:      2,
				Author:      "alice",
				AuthorEmail: "alice@example.com",
				Content:     "parent",
				CreatedAt:   1,
			},
			Children: []*site.DisplayComment{
				{
					Comment: db.Comment{
						ID:          102,
						PostID:      2,
						ParentID:    101,
						Author:      "bob",
						AuthorEmail: "bob@example.com",
						Content:     "child",
						CreatedAt:   2,
					},
					ParentAuthor: "alice",
				},
			},
		},
	}

	rendered := mustRenderRegressionTemplate(t, view, "site/post.html", map[string]any{
		"Post":                   post,
		"ReadUV":                 0,
		"LikeCount":              0,
		"Liked":                  false,
		"Comments":               comments,
		"CommentCount":           2,
		"CommentFeedback":        "",
		"CommentForm":            map[string]any{},
		"CommentCaptchaRequired": false,
		"CommentCaptcha":         map[string]any{},
		"UrlPath":                "/with-comments",
		"IsLogin":                false,
	})
	if !strings.Contains(rendered, "comment-101") {
		t.Fatalf("expected parent comment rendered, got: %s", rendered)
	}
	if !strings.Contains(rendered, "comment-102") {
		t.Fatalf("expected child comment rendered, got: %s", rendered)
	}
}

func TestRenderDashAssetsIndexWithItems(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/assets_index.html", map[string]any{
		"Items": []db.Asset{
			{
				ID:           11,
				Kind:         db.AssetKindImage,
				Provider:     "see",
				OriginalName: "cover.png",
				FileURL:      "https://example.com/cover.png",
				SizeBytes:    1234,
				CreatedAt:    1,
			},
		},
		"Pager":                 types.Pagination{Page: 1, Num: 1, Total: 1, PageSize: 10},
		"DefaultProvider":       "see",
		"DefaultProviderReady":  true,
		"DefaultProviderError":  "",
		"AssetProviderLabelMap": map[string]string{"see": "S.EE"},
		"AssetKindLabelMap":     map[string]string{"image": "图片", "file": "文件", "backup": "备份"},
		"KindCounts":            map[string]int{"image": 1, "file": 0, "backup": 0},
		"CurrentKind":           "image",
		"Query":                 map[string]string{"pageSize": "10"},
	})
	if !strings.Contains(rendered, `data-asset-id="11"`) {
		t.Fatalf("expected asset row id rendered, got: %s", rendered)
	}
	if !strings.Contains(rendered, "cover.png") {
		t.Fatalf("expected asset file name rendered, got: %s", rendered)
	}
	if strings.Contains(rendered, "暂无资源") {
		t.Fatalf("expected non-empty asset table, got: %s", rendered)
	}
	if !strings.Contains(rendered, `data-role="ui-file-upload"`) {
		t.Fatalf("expected assets page to use standard file upload component, got: %s", rendered)
	}
	if !strings.Contains(rendered, "via S.EE") {
		t.Fatalf("expected assets page upload desc to include provider label, got: %s", rendered)
	}
	if strings.Contains(rendered, `asset-upload-status`) {
		t.Fatalf("expected assets page to remove upload status panel, got: %s", rendered)
	}
	if !strings.Contains(rendered, "全部") {
		t.Fatalf("expected assets page to render all-assets tab, got: %s", rendered)
	}
}

func TestRenderDashPostsNewShowsError(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/posts_new.html", map[string]any{
		"Error":               "slug already exists",
		"DraftTitle":          "hello",
		"DraftSlug":           "hello",
		"DraftContent":        "world",
		"DraftKind":           "0",
		"DraftCategoryID":     int64(0),
		"DraftCommentEnabled": true,
		"SelectedTagNames":    "",
		"CategoryOptions":     []map[string]any{},
	})
	if !strings.Contains(rendered, "slug already exists") {
		t.Fatalf("expected post editor error message rendered, got: %s", rendered)
	}
	if !strings.Contains(rendered, `window.DashAppUI.toast.show({`) {
		t.Fatalf("expected post editor to use sui toast api, got: %s", rendered)
	}
	if !strings.Contains(rendered, `"保存失败"`) {
		t.Fatalf("expected post editor error toast rendered, got: %s", rendered)
	}
}

func TestRenderStatusMainPaginationFallsBackToRouteContext(t *testing.T) {
	view, initURLResolver := mustLoadRegressionViewWithResolver(t)

	app := fiber.New()
	app.Get("/assets", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("dash.assets.list")
	initURLResolver(app)

	rendered := mustRenderRegressionTemplate(t, view, "dash/include/status_main_pagination.html", map[string]any{
		"Pager":     types.Pagination{Page: 2, Num: 3, Total: 25, PageSize: 10},
		"RouteName": "dash.assets.list",
		"Query":     map[string]string{"kind": "image", "pageSize": "10"},
	})
	if !strings.Contains(rendered, `&#x2f;assets?kind=image&amp;page=1&amp;pageSize=10`) {
		t.Fatalf("expected pagination prev link to use route context, got: %s", rendered)
	}
	if !strings.Contains(rendered, `&#x2f;assets?kind=image&amp;page=3&amp;pageSize=10`) {
		t.Fatalf("expected pagination next link to use route context, got: %s", rendered)
	}
}

func TestRenderPaginationFallsBackToRouteContext(t *testing.T) {
	view, initURLResolver := mustLoadRegressionViewWithResolver(t)

	app := fiber.New()
	app.Get("/logs", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("dash.http_error_logs.list")
	initURLResolver(app)

	rendered := mustRenderRegressionTemplate(t, view, "dash/include/pagination.html", map[string]any{
		"Pager":     types.Pagination{Page: 2, Num: 3, Total: 25, PageSize: 10},
		"RouteName": "dash.http_error_logs.list",
		"Query":     map[string]string{"pageSize": "10"},
	})
	if !strings.Contains(rendered, `&#x2f;logs?page=1&amp;pageSize=10`) {
		t.Fatalf("expected pagination prev link to use route context, got: %s", rendered)
	}
	if !strings.Contains(rendered, `&#x2f;logs?page=3&amp;pageSize=10`) {
		t.Fatalf("expected pagination next link to use route context, got: %s", rendered)
	}
}

func TestRenderDashImportWithoutFeedback(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/import.html", map[string]any{
		"ImportingItems": []dash.PreviewPostItem{},
		"ImportingTotal": 0,
		"Pager":          types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 20},
		"AllCategories":  []db.Category{},
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, `data-import-edit-status`) {
		t.Fatalf("expected import row status cell")
	}
	if !strings.Contains(rendered, `data-import-row-retry-btn`) {
		t.Fatalf("expected import retry button markup")
	}
	if !strings.Contains(rendered, `data-role="ui-file-upload"`) {
		t.Fatalf("expected import page to use standard file upload component")
	}
}

func TestRenderDashBackupRestoreShowsRestoreControls(t *testing.T) {
	view, initURLResolver := mustLoadRegressionViewWithResolver(t)

	app := fiber.New()
	app.Get("/dash/backup-restore", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.show")
	app.Get("/dash/backup-restore/status", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.status")
	app.Post("/dash/backup-restore/backup", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.backup")
	app.Post("/dash/backup-restore/local", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.local")
	app.Post("/dash/backup-restore/upload", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.upload")
	app.Post("/dash/backup-restore/delete", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.backup_restore.delete")
	initURLResolver(app)

	rendered := mustRenderRegressionTemplate(t, view, "dash/backup_restore.html", map[string]any{
		"RestoreStatusLabel":  "空闲",
		"RestoreStatusKind":   "info",
		"RestoreStatus":       "idle",
		"RestoreEnabled":      true,
		"RestoreStatusAPIURL": "/dash/backup-restore/status",
		"LocalBackupDir":      "backups",
		"Pager":               types.Pagination{Page: 1, PageSize: 10, Num: 2, Total: 11},
		"LocalBackupFiles": []map[string]any{
			{"Name": "2026-04-08.sqlite", "ModifiedAt": time.Now().Add(-2 * time.Hour).Unix(), "Size": 1024},
		},
	})
	if !strings.Contains(rendered, "本地备份文件列表") {
		t.Fatalf("expected local restore section in backup restore view")
	}
	if !strings.Contains(rendered, "执行本地备份") {
		t.Fatalf("expected local backup action in backup restore view")
	}
	if !strings.Contains(rendered, `data-title="恢复"`) {
		t.Fatalf("expected backup restore action in backup restore view")
	}
	if !strings.Contains(rendered, `class="cell-checkbox ui-checkbox"`) {
		t.Fatalf("expected backup restore multiselect checkboxes in backup restore view")
	}
	if !strings.Contains(rendered, `确定删除这个本地备份文件吗`) {
		t.Fatalf("expected backup delete action in backup restore view")
	}
	if strings.Contains(rendered, "从选中文件恢复") {
		t.Fatalf("expected selected backup restore button removed from backup restore view")
	}
	if !strings.Contains(rendered, `data-role="ui-file-upload"`) {
		t.Fatalf("expected standard file upload component in backup restore view")
	}
	if !strings.Contains(rendered, `backup-restore-confirm-dialog`) {
		t.Fatalf("expected sui confirm dialog in backup restore view")
	}
	if !strings.Contains(rendered, `aria-label="分页"`) {
		t.Fatalf("expected backup restore pagination link in backup restore view")
	}
	if strings.Contains(rendered, "上传并恢复") {
		t.Fatalf("expected upload restore button removed from backup restore view")
	}
	if !strings.Contains(rendered, "1 KB") {
		t.Fatalf("expected human readable backup size in backup restore view")
	}
	if !strings.Contains(rendered, "小时前") {
		t.Fatalf("expected relative backup modified time in backup restore view")
	}
	if strings.Contains(rendered, `name="confirm_text"`) {
		t.Fatalf("expected backup restore view to remove confirm text input")
	}
}

func TestRenderDashImportShowsExportTab(t *testing.T) {
	view, initURLResolver := mustLoadRegressionViewWithResolver(t)

	app := fiber.New()
	app.Get("/dash/import", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.import.show")
	app.Get("/dash/export/download", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }).Name("dash.export.download")
	initURLResolver(app)

	rendered := mustRenderRegressionTemplate(t, view, "dash/import.html", map[string]any{
		"ImportExportTab": "export",
		"ImportingItems":  []dash.PreviewPostItem{},
		"ImportingTotal":  0,
		"Pager":           types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 20},
		"AllCategories":   []db.Category{},
	})
	if !strings.Contains(rendered, "导入导出") {
		t.Fatalf("expected import/export tabs heading")
	}
	if !strings.Contains(rendered, "数据库导出") {
		t.Fatalf("expected export panel in import export tab")
	}
}

func TestRenderDashHttpErrorLogsShowsAddRedirectActionForGet404(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/http_error_logs_index.html", map[string]any{
		"Logs": []db.HttpErrorLog{
			{
				ID:        1,
				Method:    "GET",
				Path:      "/missing-path",
				Status:    404,
				CreatedAt: 1,
			},
			{
				ID:        2,
				Method:    "POST",
				Path:      "/missing-post",
				Status:    404,
				CreatedAt: 1,
			},
		},
		"Pager": types.Pagination{Page: 1, Num: 1, Total: 2, PageSize: 10},
	})
	if count := strings.Count(rendered, "lucide-arrow-right-icon"); count != 1 {
		t.Fatalf("expected add-redirect icon once, got %d", count)
	}
}

func TestRenderDashRedirectsNewShowsTargetPicker(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/redirects_new.html", map[string]any{
		"Redirect": db.Redirect{
			From:    "/missing-path",
			To:      "",
			Status:  301,
			Enabled: 1,
		},
		"RedirectTargetOptions": []map[string]any{
			{
				"ID":        int64(11),
				"Title":     "Hello World",
				"URL":       "/posts/hello-world",
				"KindLabel": "文章",
			},
		},
	})
	if !strings.Contains(rendered, "选择文章 URL") {
		t.Fatalf("expected target picker entry in redirects_new")
	}
	if !strings.Contains(rendered, "Hello World") {
		t.Fatalf("expected target option title in redirects_new")
	}
	if !strings.Contains(rendered, `id="redirect-target-picker-open"`) {
		t.Fatalf("expected target picker open trigger in redirects_new")
	}
	if !strings.Contains(rendered, `id="redirect-target-picker-search"`) {
		t.Fatalf("expected target picker search input in redirects_new")
	}
	if !strings.Contains(rendered, `data-role="redirect-target-picker-choose"`) {
		t.Fatalf("expected target picker choose action in redirects_new")
	}
	if !strings.Contains(rendered, `data-target-url="/posts/hello-world"`) {
		t.Fatalf("expected target picker option url in redirects_new")
	}
	if !strings.Contains(rendered, `id="redirect-target-picker-empty"`) {
		t.Fatalf("expected target picker empty state in redirects_new")
	}
	dialogStart := strings.Index(rendered, `id="redirect-target-picker-dialog"`)
	if dialogStart < 0 {
		t.Fatalf("expected target picker dialog markup in redirects_new")
	}
	dialogEnd := dialogStart + 2000
	if dialogEnd > len(rendered) {
		dialogEnd = len(rendered)
	}
	dialogSnippet := rendered[dialogStart:dialogEnd]
	if count := strings.Count(dialogSnippet, `data-role="ui-dialog-close"`); count != 1 {
		t.Fatalf("expected exactly one dialog close action, got %d", count)
	}
}

func TestRenderDashRedirectsIndexShowsImportAction(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/redirects_index.html", map[string]any{
		"Redirects": []db.Redirect{},
		"Pager":     types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 10},
	})
	if !strings.Contains(rendered, "导入重定向") {
		t.Fatalf("expected import action button in redirects_index")
	}
	if !strings.Contains(rendered, `id="redirect-import-form"`) {
		t.Fatalf("expected import form in redirects_index")
	}
	if !strings.Contains(rendered, `id="redirect-import-file"`) {
		t.Fatalf("expected import file input in redirects_index")
	}
}

func TestRenderDashRedirectsCreateRouteKeepsSaveAction(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/redirects_new.html", map[string]any{
		"RouteName": "dash.redirects.create",
		"Redirect": db.Redirect{
			From:    "/missing-path",
			To:      "",
			Status:  301,
			Enabled: 1,
		},
	})
	if !strings.Contains(rendered, `type="submit" form="form"`) {
		t.Fatalf("expected save action button for configured create route")
	}
}

func TestRenderDashSettingsSystemUpdateShowsRestartAction(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/settings_system_update.html", map[string]any{
		"FrontendArea": dash.SettingAreaView{},
		"BackendArea": dash.SettingAreaView{
			Code: "backend",
			Sections: []dash.SettingSectionView{
				{Code: "general", Label: "常规", Description: "desc", SettingCount: 1},
			},
		},
		"CurrentVersion":      "v1.0.0",
		"LatestVersion":       "v1.0.1",
		"HasSystemUpdate":     true,
		"AutoUpdateEnabled":   true,
		"ManualUpdateEnabled": true,
		"RestartEnabled":      true,
	})
	if !strings.Contains(rendered, "系统更新") {
		t.Fatalf("expected system update label in settings_system_update")
	}
	if !strings.Contains(rendered, "系统重启") {
		t.Fatalf("expected restart button in settings_system_update")
	}
	if !strings.Contains(rendered, "system-update-restart-form") {
		t.Fatalf("expected restart form in settings_system_update")
	}
}

func TestRenderDashSettingsAllWithSettingView(t *testing.T) {
	view := mustLoadRegressionView(t)

	areas := []dash.SettingAreaView{
		{
			Code:  "frontend",
			Label: "前台",
			Sections: []dash.SettingSectionView{
				{
					Code:        "site",
					Label:       "站点信息",
					Description: "配置公开站点的名称、访问地址、语言和页面基础信息。",
					Cards: []dash.SettingCardView{
						{
							Code:  "identity",
							Label: "基础信息",
							Settings: []dash.SettingView{
								{
									Setting: db.Setting{
										Kind:  "site",
										Name:  "站点标题",
										Code:  "site_title",
										Type:  "text",
										Value: "Swaves",
									},
									AttrsParsed: map[string]any{},
								},
							},
						},
					},
				},
			},
		},
	}

	rendered := mustRenderRegressionTemplate(t, view, "dash/settings_all.html", map[string]any{
		"SettingAreas":              areas,
		"ActiveArea":                areas[0],
		"ActiveSection":             areas[0].Sections[0],
		"ContentRoutingSectionCode": "content",
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, "站点信息") {
		t.Fatalf("expected rendered settings output to contain section title")
	}
	if !strings.Contains(rendered, "前台") {
		t.Fatalf("expected rendered settings output to contain area label")
	}
}

func TestRenderDashMonitorWithMapGranularities(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/monitor.html", map[string]any{
		"Granularities": []map[string]any{
			{"Key": "1m", "Label": "1分钟"},
		},
		"ActiveGranularity": "1m",
		"ActiveScope":       "app",
	})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, "function bindChartTooltips()") {
		t.Fatalf("expected monitor page to include chart tooltip binding logic")
	}
}

func TestRenderDashPostsEditContainsSEditorMount(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "dash/posts_edit.html", map[string]any{
		"SEditor": true,
		"Post": db.Post{
			ID:             2,
			Title:          "hello",
			Slug:           "hello",
			Content:        "content",
			Status:         "draft",
			Kind:           db.PostKindPost,
			CommentEnabled: 1,
		},
		"Category":         db.Category{ID: 0},
		"Categories":       []db.Category{},
		"SelectedTagNames": "",
	})
	if !strings.Contains(rendered, `class="content wysiwyg"`) {
		t.Fatalf("expected rendered output to contain editor mount container")
	}
	if !strings.Contains(rendered, `/static/seditor/dist/seditor.min.js`) {
		t.Fatalf("expected rendered output to contain seditor script include")
	}
}

func TestRenderSiteHomeWithDisplayPosts(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "site/home.html", map[string]any{
		"Articles": []site.DisplayPost{
			{
				Post: db.Post{
					Title:       "home-title",
					PublishedAt: 1,
				},
				PermLink: "/hello",
			},
		},
		"Pager": types.Pagination{Page: 1, Num: 1, Total: 1, PageSize: 10},
	})
	if !strings.Contains(rendered, "home-title") {
		t.Fatalf("expected rendered article title")
	}
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("expected rendered article permalink, got: %s", rendered)
	}
}

func TestRenderSiteDetailWithTagContextOnly(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "site/detail.html", map[string]any{
		"IsTag": true,
		"Entity": site.DisplayItem{
			ID:          7,
			Name:        "golang",
			Description: "tag desc",
			PermLink:    "/tag/golang",
		},
		"List": []site.DisplayPostRelativeInfo{
			{
				ID:          11,
				Title:       "article-a",
				PermLink:    "/article-a",
				CreatedAt:   1,
				Tags:        []site.DisplayItem{{Name: "golang", PermLink: "/tag/golang"}},
				Category:    &site.DisplayItem{Name: "dev", PermLink: "/category/dev"},
				PublishedAt: 1,
			},
		},
	})
	if !strings.Contains(rendered, "golang") {
		t.Fatalf("expected rendered tag name")
	}
	if !strings.Contains(rendered, "article-a") {
		t.Fatalf("expected rendered article title")
	}
}

func TestRenderLucideIconWithoutSize(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "site/include/read_uv.html", map[string]any{"Count": 0})
	if !strings.Contains(rendered, "<svg") {
		t.Fatalf("expected svg output")
	}
}

func TestRenderLucideNewspaperIcon(t *testing.T) {
	rendered := renderLucideIconSVG("newspaper", "24")
	if !strings.Contains(rendered, `data-name="newspaper"`) {
		t.Fatalf("expected newspaper data-name, got %q", rendered)
	}
	if !strings.Contains(rendered, "lucide-newspaper") {
		t.Fatalf("expected newspaper svg class, got %q", rendered)
	}
}

func TestRenderSiteLayoutWithoutTitle(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "site/layout/layout.html", map[string]any{})
	if rendered == "" {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(rendered, `/static/favicon.svg?v=2`) {
		t.Fatalf("expected favicon link in site layout")
	}
	if strings.Contains(rendered, `/static/katex/katex.min.css`) {
		t.Fatalf("expected site layout not to include math assets by default")
	}
}

func TestRenderSiteLayoutUsesSiteTitleFallback(t *testing.T) {
	view := mustLoadRegressionView(t)

	withRegressionSettings(t, map[string]string{"site_title": "Example Site"})

	rendered := mustRenderRegressionTemplate(t, view, "site/layout/layout.html", map[string]any{})
	if !strings.Contains(rendered, "<title>Example Site</title>") {
		t.Fatalf("expected site title fallback, got %s", rendered)
	}
}

func TestRenderSiteLayoutIncludesCanonicalAndDescription(t *testing.T) {
	view := mustLoadRegressionView(t)

	html := mustRenderRegressionTemplate(t, view, "site/layout/layout.html", map[string]any{
		"Title":           "Hello",
		"CanonicalURL":    "https://example.com/hello",
		"MetaDescription": "Hello description",
	})
	if !strings.Contains(html, `rel="canonical"`) || !strings.Contains(html, `example.com`) {
		t.Fatalf("expected canonical tag, got %s", html)
	}
	if !strings.Contains(html, `<meta name="description" content="Hello description"/>`) {
		t.Fatalf("expected description meta tag, got %s", html)
	}
}

func TestRenderSUILayoutIncludesFavicon(t *testing.T) {
	view := mustLoadRegressionView(t)

	rendered := mustRenderRegressionTemplate(t, view, "sui/layout/base.html", map[string]any{})
	if !strings.Contains(rendered, `/static/favicon.svg?v=2`) {
		t.Fatalf("expected favicon link in sui layout")
	}
}

func TestRenderMonitorJSURLsAreNotHTMLEscaped(t *testing.T) {
	view := newMiniJinjaView(testTemplateRoot(), false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		switch name {
		case "dash.monitor.data":
			return "/dash/api/monitor"
		case "dash.monitor":
			return "/dash/monitor"
		default:
			return "/"
		}
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	rendered := mustRenderRegressionTemplate(t, view, "dash/monitor.html", map[string]any{
		"Granularities": []map[string]any{
			{"Key": "1m", "Label": "1分钟"},
		},
		"ActiveGranularity": "1m",
		"ActiveScope":       "app",
	})
	if strings.Contains(rendered, "var monitorAPIURL = '&#x2f;") {
		t.Fatalf("expected monitor js api url not to be html escaped")
	}
	if !strings.Contains(rendered, `var monitorAPIURL = "/dash/api/monitor";`) {
		t.Fatalf("expected monitor api url in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, `var monitorPageURL = "/dash/monitor";`) {
		t.Fatalf("expected monitor base url in output, got: %s", rendered)
	}
}

func TestRenderImportJSURLsAndCategoryOptionsAreNotHTMLEscaped(t *testing.T) {
	view := newMiniJinjaView(testTemplateRoot(), false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		switch name {
		case "dash.import.submit":
			return "/dash/import"
		case "dash.import.parse_item":
			return "/dash/import/parse-item"
		case "dash.import.confirm_item":
			return "/dash/import/confirm-item"
		case "dash.import.cancel":
			return "/dash/import/cancel"
		case "dash.posts.list":
			return "/dash/posts"
		default:
			return "/"
		}
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	rendered := mustRenderRegressionTemplate(t, view, "dash/import.html", map[string]any{
		"ImportingItems": []dash.PreviewPostItem{},
		"ImportingTotal": 0,
		"Pager":          types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 20},
		"AllCategories": []db.Category{
			{Name: "生活"},
			{Name: "文娱"},
		},
	})
	if strings.Contains(rendered, "var parseItemURL = '&#x2f;dash&#x2f;import&#x2f;parse-item';") {
		t.Fatalf("expected parse-item url in js not to be html escaped")
	}
	if strings.Contains(rendered, "&quot;生活&quot;") {
		t.Fatalf("expected category options in js not to be html escaped")
	}
	if !strings.Contains(rendered, `var parseItemURL = "/dash/import/parse-item";`) {
		t.Fatalf("expected parse-item url in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, `"生活"`) {
		t.Fatalf("expected category option in output, got: %s", rendered)
	}
}
