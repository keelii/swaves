package view

import (
	"bytes"
	"strings"
	"testing"

	dash "swaves/internal/modules/dash"
	"swaves/internal/modules/site"
	"swaves/internal/platform/db"
	"swaves/internal/shared/types"
)

func TestRenderDashCategoriesIndexWithMissingCounts(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/categories_index.html", map[string]any{
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
	if err != nil {
		t.Fatalf("render categories index failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderDashPostsIndexWithoutFilterNames(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/posts_index.html", map[string]any{
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
	view, _ := NewViewEngine(testTemplateRoot(), false)
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
	err := view.Render(&out, "site/post.html", map[string]any{
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
	view, _ := NewViewEngine(testTemplateRoot(), false)
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
	err := view.Render(&out, "site/post.html", map[string]any{
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

func TestRenderDashAssetsIndexWithItems(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/assets_index.html", map[string]any{
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

func TestRenderDashImportWithoutFeedback(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/import.html", map[string]any{
		"ImportingItems": []dash.PreviewPostItem{},
		"ImportingTotal": 0,
		"Pager":          types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 20},
		"AllCategories":  []db.Category{},
	})
	if err != nil {
		t.Fatalf("render import failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
}

func TestRenderDashHttpErrorLogsShowsAddRedirectActionForGet404(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/http_error_logs_index.html", map[string]any{
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

func TestRenderDashRedirectsNewShowsTargetPicker(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/redirects_new.html", map[string]any{
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
	if !strings.Contains(rendered, "redirect-target-picker-choose") {
		t.Fatalf("expected target picker choose button in redirects_new")
	}
}

func TestRenderDashRedirectsCreateRouteKeepsSaveAction(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/redirects_new.html", map[string]any{
		"RouteName": "dash.redirects.create",
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
	if !strings.Contains(rendered, `type="submit" form="form"`) {
		t.Fatalf("expected save action button for configured create route")
	}
}

func TestRenderDashSettingsAllWithSettingView(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

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

	var out bytes.Buffer
	err := view.Render(&out, "dash/settings_all.html", map[string]any{
		"SettingAreas":              areas,
		"ActiveArea":                areas[0],
		"ActiveSection":             areas[0].Sections[0],
		"ContentRoutingSectionCode": "content",
	})
	if err != nil {
		t.Fatalf("render settings_all failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
	rendered := out.String()
	if !strings.Contains(rendered, "站点信息") {
		t.Fatalf("expected rendered settings output to contain section title")
	}
	if !strings.Contains(rendered, "前台") {
		t.Fatalf("expected rendered settings output to contain area label")
	}
}

func TestRenderDashMonitorWithMapGranularities(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/monitor.html", map[string]any{
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
	rendered := out.String()
	if !strings.Contains(rendered, "function bindChartTooltips()") {
		t.Fatalf("expected monitor page to include chart tooltip binding logic")
	}
}

func TestRenderDashPostsEditContainsSEditorMount(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "dash/posts_edit.html", map[string]any{
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
	if err != nil {
		t.Fatalf("render posts_edit failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, `class="content wysiwyg"`) {
		t.Fatalf("expected rendered output to contain editor mount container")
	}
	if !strings.Contains(rendered, `/static/seditor/dist/seditor.min.js`) {
		t.Fatalf("expected rendered output to contain seditor script include")
	}
}

func TestRenderSiteHomeWithDisplayPosts(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/home.html", map[string]any{
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
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/detail.html", map[string]any{
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
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/include/read_uv.html", map[string]any{"Count": 0})
	if err != nil {
		t.Fatalf("render lucide icon failed: %v", err)
	}
	if !strings.Contains(out.String(), "<svg") {
		t.Fatalf("expected svg output")
	}
}

func TestRenderSiteLayoutWithoutTitle(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "site/layout/layout.html", map[string]any{})
	if err != nil {
		t.Fatalf("render site layout failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected non-empty render output")
	}
	if !strings.Contains(out.String(), `/static/favicon.svg`) {
		t.Fatalf("expected favicon link in site layout")
	}
}

func TestRenderSUILayoutIncludesFavicon(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "sui/layout/base.html", map[string]any{})
	if err != nil {
		t.Fatalf("render sui layout failed: %v", err)
	}
	if !strings.Contains(out.String(), `/static/favicon.svg`) {
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

	var out bytes.Buffer
	err := view.Render(&out, "dash/monitor.html", map[string]any{
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

	var out bytes.Buffer
	err := view.Render(&out, "dash/import.html", map[string]any{
		"ImportingItems": []dash.PreviewPostItem{},
		"ImportingTotal": 0,
		"Pager":          types.Pagination{Page: 1, Num: 1, Total: 0, PageSize: 20},
		"AllCategories": []db.Category{
			{Name: "生活"},
			{Name: "文娱"},
		},
	})
	if err != nil {
		t.Fatalf("render import failed: %v", err)
	}

	rendered := out.String()
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
