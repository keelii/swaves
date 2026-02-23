package view

import (
	"bytes"
	"strings"
	"testing"

	"swaves/internal/modules/admin"
	"swaves/internal/modules/site"
	"swaves/internal/platform/db"
	"swaves/internal/shared/types"
)

func TestRenderAdminCategoriesIndexWithMissingCounts(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/categories_index", map[string]any{
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
	})
	if err != nil {
		t.Fatalf("render categories index failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderAdminPostsIndexWithoutFilterNames(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/posts_index", map[string]any{
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
	if err != nil {
		t.Fatalf("render posts index failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderSitePostWithEmbeddedDisplayPost(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

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

	var out bytes.Buffer
	err := view.Render(&out, "site/post", map[string]any{
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
	if err != nil {
		t.Fatalf("render site post failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
	rendered := out.String()
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("expected rendered post title/content")
	}
	if !strings.Contains(rendered, "<p>hello</p>") {
		t.Fatalf("expected rendered html content")
	}
}

func TestRenderSitePostWithCommentTree(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

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

	var out bytes.Buffer
	err := view.Render(&out, "site/post", map[string]any{
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
	if err != nil {
		t.Fatalf("render site post with comments failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "comment-101") {
		t.Fatalf("expected parent comment rendered, got: %s", rendered)
	}
	if !strings.Contains(rendered, "comment-102") {
		t.Fatalf("expected child comment rendered, got: %s", rendered)
	}
}

func TestRenderAdminAssetsIndexWithItems(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/assets_index", map[string]any{
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
		"AssetKindLabelMap":     map[string]string{"image": "图片"},
	})
	if err != nil {
		t.Fatalf("render assets index failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `data-asset-id="11"`) {
		t.Fatalf("expected asset row id rendered, got: %s", rendered)
	}
	if !strings.Contains(rendered, "cover.png") {
		t.Fatalf("expected asset file name rendered, got: %s", rendered)
	}
	if strings.Contains(rendered, "暂无资源") {
		t.Fatalf("expected non-empty asset table, got: %s", rendered)
	}
}

func TestRenderAdminImportWithoutFeedback(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/import", map[string]any{
		"ImportingItems": []admin.PreviewPostItem{},
		"AllCategories":  []db.Category{},
	})
	if err != nil {
		t.Fatalf("render import failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderAdminHttpErrorLogsShowsAddRedirectActionForGet404(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/http_error_logs_index", map[string]any{
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
	if err != nil {
		t.Fatalf("render http_error_logs_index failed: %v", err)
	}

	rendered := out.String()
	if count := strings.Count(rendered, "lucide-arrow-right-icon"); count != 1 {
		t.Fatalf("expected add-redirect icon once, got %d", count)
	}
}

func TestRenderAdminRedirectsNewShowsTargetPicker(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/redirects_new", map[string]any{
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
	if err != nil {
		t.Fatalf("render redirects_new failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "选择文章 URL") {
		t.Fatalf("expected target picker entry in redirects_new")
	}
	if !strings.Contains(rendered, "Hello World") {
		t.Fatalf("expected target option title in redirects_new")
	}
	if !strings.Contains(rendered, "redirect_target_picker_choose") {
		t.Fatalf("expected target picker choose button in redirects_new")
	}
	if !strings.Contains(rendered, "lucide-archive-icon") {
		t.Fatalf("expected archive icon for redirects target picker trigger")
	}
}

func TestRenderAdminRedirectsCreateRouteKeepsSaveAction(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/redirects_new", map[string]any{
		"RouteName": "admin.redirects.create",
		"Redirect": db.Redirect{
			From:    "/missing-path",
			To:      "",
			Status:  301,
			Enabled: 1,
		},
	})
	if err != nil {
		t.Fatalf("render redirects_new failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "document.querySelector('form').submit()") {
		t.Fatalf("expected save action button for admin.redirects.create route")
	}
}

func TestRenderAdminSettingsAllWithSettingView(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	groups := []admin.SettingSubKindGroupView{
		{
			Code:  "",
			Label: "",
			Settings: []admin.SettingView{
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
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/settings_all", map[string]any{
		"SettingKinds":       []string{"site"},
		"SettingKindLabels":  map[string]string{"site": "站点"},
		"ActiveKind":         "site",
		"ActiveKindGroups":   groups,
		"ContentRoutingKind": db.SettingKindContentRouting,
	})
	if err != nil {
		t.Fatalf("render settings_all failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderAdminMonitorWithMapGranularities(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/monitor", map[string]any{
		"Granularities": []map[string]any{
			{"Key": "1m", "Label": "1分钟"},
		},
		"ActiveGranularity": "1m",
		"ActiveScope":       "app",
	})
	if err != nil {
		t.Fatalf("render monitor failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderAdminPostsEditUsesAssetAPIPath(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/posts_edit", map[string]any{
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
	if err != nil {
		t.Fatalf("render posts_edit failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "/api/assets") {
		t.Fatalf("expected rendered output to contain asset api path")
	}
	if strings.Contains(rendered, "admin/posts/2/&") {
		t.Fatalf("unexpected malformed asset request base in rendered output")
	}
}

func TestRenderSiteHomeWithDisplayPosts(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/home", map[string]any{
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
	if err != nil {
		t.Fatalf("render site home failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "home-title") {
		t.Fatalf("expected rendered article title")
	}
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("expected rendered article permalink, got: %s", rendered)
	}
}

func TestRenderSiteDetailWithTagContextOnly(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/detail", map[string]any{
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
	if err != nil {
		t.Fatalf("render site detail failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "golang") {
		t.Fatalf("expected rendered tag name")
	}
	if !strings.Contains(rendered, "article-a") {
		t.Fatalf("expected rendered article title")
	}
}

func TestRenderLucideIconWithoutSize(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/include/read_uv", map[string]any{"Count": 0})
	if err != nil {
		t.Fatalf("render lucide icon failed: %v", err)
	}
	if !strings.Contains(out.String(), "<svg") {
		t.Fatalf("expected svg output")
	}
}

func TestRenderSiteLayoutWithoutTitle(t *testing.T) {
	view, _, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/layout/layout", map[string]any{})
	if err != nil {
		t.Fatalf("render site layout failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderMonitorJSURLsAreNotHTMLEscaped(t *testing.T) {
	view := newMiniJinjaView(testTemplateRoot(), false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		switch name {
		case "admin.monitor.data":
			return "/admin/api/monitor"
		case "admin.monitor":
			return "/admin/monitor"
		default:
			return "/"
		}
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/monitor", map[string]any{
		"Granularities": []map[string]any{
			{"Key": "1m", "Label": "1分钟"},
		},
		"ActiveGranularity": "1m",
		"ActiveScope":       "app",
	})
	if err != nil {
		t.Fatalf("render monitor failed: %v", err)
	}
	rendered := out.String()
	if strings.Contains(rendered, "var monitorAPIURL = '&#x2f;") {
		t.Fatalf("expected monitor js api url not to be html escaped")
	}
	if strings.Contains(rendered, "buildURL('&#x2f;") {
		t.Fatalf("expected monitor js base url not to be html escaped")
	}
	if !strings.Contains(rendered, `var monitorAPIURL = "/admin/api/monitor";`) {
		t.Fatalf("expected monitor api url in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, `buildURL("/admin/monitor", {`) {
		t.Fatalf("expected monitor base url in output, got: %s", rendered)
	}
}

func TestRenderImportJSURLsAndCategoryOptionsAreNotHTMLEscaped(t *testing.T) {
	view := newMiniJinjaView(testTemplateRoot(), false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		switch name {
		case "admin.import.submit":
			return "/admin/import"
		case "admin.import.parse_item":
			return "/admin/import/parse-item"
		case "admin.import.confirm_item":
			return "/admin/import/confirm-item"
		case "admin.import.cancel":
			return "/admin/import/cancel"
		case "admin.posts.list":
			return "/admin/posts"
		default:
			return "/"
		}
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "admin/import", map[string]any{
		"ImportingItems": []admin.PreviewPostItem{},
		"AllCategories": []db.Category{
			{Name: "生活"},
			{Name: "文娱"},
		},
	})
	if err != nil {
		t.Fatalf("render import failed: %v", err)
	}

	rendered := out.String()
	if strings.Contains(rendered, "var parseItemURL = '&#x2f;admin&#x2f;import&#x2f;parse-item';") {
		t.Fatalf("expected parse-item url in js not to be html escaped")
	}
	if strings.Contains(rendered, "&quot;生活&quot;") {
		t.Fatalf("expected category options in js not to be html escaped")
	}
	if !strings.Contains(rendered, `var parseItemURL = "/admin/import/parse-item";`) {
		t.Fatalf("expected parse-item url in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, `"生活"`) {
		t.Fatalf("expected category option in output, got: %s", rendered)
	}
}
