package dash

import (
	"testing"

	"swaves/internal/platform/db"
)

func findSettingArea(areas []SettingAreaView, code string) *SettingAreaView {
	for idx := range areas {
		if areas[idx].Code == code {
			return &areas[idx]
		}
	}
	return nil
}

func findSettingSection(area *SettingAreaView, code string) *SettingSectionView {
	if area == nil {
		return nil
	}
	for idx := range area.Sections {
		if area.Sections[idx].Code == code {
			return &area.Sections[idx]
		}
	}
	return nil
}

func countSectionSettings(section *SettingSectionView) int {
	if section == nil {
		return 0
	}
	total := 0
	for _, card := range section.Cards {
		total += len(card.Settings)
	}
	return total
}

func TestBuildSettingAreasGroupsCrossKindSettings(t *testing.T) {
	areas := buildSettingAreas([]db.Setting{
		{Kind: db.SettingKindSiteBasics, Name: "访问地址", Code: "site_url", Type: "text", Value: "https://example.com"},
		{Kind: db.SettingKindDashSecurity, Name: "资源默认服务", Code: "asset_default_provider", Type: "select", Value: "see"},
		{Kind: db.SettingKindThirdPartyServices, Name: "S.EE API 地址", Code: "asset_see_api_base", Type: "url", Value: "https://s.ee/api/v1/file/upload"},
		{Kind: db.SettingKindThirdPartyServices, Name: "S3 接口地址", Code: "s3_api_endpoint", Type: "url", Value: "https://s3.example.com"},
		{Kind: db.SettingKindThirdPartyServices, Name: "Google analytics ID", Code: "ga4_id", Type: "text", Value: "G-TEST"},
		{Kind: db.SettingKindUIExperience, Name: "文字大小", Code: "font_size", Type: "number", Value: "14"},
	})

	frontend := findSettingArea(areas, settingAreaFrontend)
	if frontend == nil {
		t.Fatalf("expected frontend area to exist")
	}
	site := findSettingSection(frontend, settingSectionSite)
	if site == nil {
		t.Fatalf("expected site section to exist")
	}
	if got := countSectionSettings(site); got != 1 {
		t.Fatalf("expected site section to include 1 setting, got %d", got)
	}

	integrations := findSettingSection(frontend, settingSectionIntegrations)
	if integrations == nil {
		t.Fatalf("expected integrations section to exist")
	}
	if got := countSectionSettings(integrations); got != 1 {
		t.Fatalf("expected integrations section to include 1 setting, got %d", got)
	}

	backend := findSettingArea(areas, settingAreaBackend)
	if backend == nil {
		t.Fatalf("expected backend area to exist")
	}
	assets := findSettingSection(backend, settingSectionAssets)
	if assets == nil {
		t.Fatalf("expected assets section to exist")
	}
	if got := countSectionSettings(assets); got != 3 {
		t.Fatalf("expected assets section to include 3 settings from different kinds, got %d", got)
	}

	backup := findSettingSection(backend, settingSectionBackup)
	if backup != nil {
		t.Fatalf("expected backup section to stay hidden without backup settings, got %+v", backup)
	}

	display := findSettingSection(frontend, settingSectionDisplay)
	if display != nil {
		t.Fatalf("expected hidden font_size setting not to create display section by itself")
	}
}

func TestResolveSettingLocationFallsBackToOtherSection(t *testing.T) {
	location := resolveSettingLocation(db.Setting{Kind: "custom", Code: "custom_setting_code"})
	if location.Area != settingAreaBackend || location.Section != settingSectionOther || location.Card != "misc" {
		t.Fatalf("unexpected fallback location: %+v", location)
	}
}

func TestBuildSettingAreasAddsSectionCountsAndSummaries(t *testing.T) {
	areas := buildSettingAreas([]db.Setting{
		{Kind: db.SettingKindSiteBasics, Name: "站点名称", Code: "site_name", Type: "text", Value: "Swaves"},
		{Kind: db.SettingKindSiteBasics, Name: "站点地址", Code: "site_url", Type: "url", Value: "https://example.com"},
		{Kind: db.SettingKindContentRouting, Name: "基础路径", Code: "base_path", Type: "text", Value: "blog"},
		{Kind: db.SettingKindContentRouting, Name: "文章前缀", Code: "post_url_prefix", Type: "text", Value: "archives"},
		{Kind: db.SettingKindContentRouting, Name: "文章名称", Code: "post_url_name", Type: "text", Value: "{slug}"},
		{Kind: db.SettingKindContentRouting, Name: "文章后缀", Code: "post_url_ext", Type: "text", Value: ".html"},
		{Kind: db.SettingKindUIExperience, Name: "界面模式", Code: "mode", Type: "radio", Value: "light"},
		{Kind: db.SettingKindDashSecurity, Name: "资源默认服务", Code: "asset_default_provider", Type: "select", Value: "imagekit"},
		{Kind: db.SettingKindBackupSync, Name: "开启远程备份", Code: "sync_push_enabled", Type: "radio", Value: "1", DefaultOptionValue: "0"},
		{Kind: db.SettingKindBackupSync, Name: "远程备份服务商", Code: "sync_push_provider", Type: "select", Value: "s3", DefaultOptionValue: "s3"},
		{Kind: db.SettingKindNotifications, Name: "文章点赞通知", Code: "notify_enable_post_like", Type: "radio", Value: "1", DefaultOptionValue: "1"},
		{Kind: db.SettingKindNotifications, Name: "用户留言通知", Code: "notify_enable_comment", Type: "radio", Value: "1", DefaultOptionValue: "1"},
		{Kind: db.SettingKindNotifications, Name: "任务成功通知", Code: "notify_enable_task_success", Type: "radio", Value: "0", DefaultOptionValue: "0"},
		{Kind: db.SettingKindNotifications, Name: "任务失败通知", Code: "notify_enable_task_error", Type: "radio", Value: "1", DefaultOptionValue: "1"},
	})

	frontend := findSettingArea(areas, settingAreaFrontend)
	if frontend == nil {
		t.Fatalf("expected frontend area to exist")
	}
	if len(frontend.Sections) == 0 || frontend.Sections[0].Code != settingSectionDisplay {
		t.Fatalf("expected frontend display section to be first, got %+v", frontend.Sections)
	}
	display := findSettingSection(frontend, settingSectionDisplay)
	if display == nil {
		t.Fatalf("expected display section to exist")
	}
	if display.Summary != "浅色界面" {
		t.Fatalf("unexpected display summary: %q", display.Summary)
	}

	site := findSettingSection(frontend, settingSectionSite)
	if site == nil {
		t.Fatalf("expected site section to exist")
	}
	if site.SettingCount != 1 {
		t.Fatalf("expected site setting count = 1, got %d", site.SettingCount)
	}
	if site.Summary != "https://example.com" {
		t.Fatalf("unexpected site summary: %q", site.Summary)
	}

	content := findSettingSection(frontend, settingSectionContent)
	if content == nil {
		t.Fatalf("expected content section to exist")
	}
	if content.Summary != "文章 /blog/archives/{slug}.html" {
		t.Fatalf("unexpected content summary: %q", content.Summary)
	}

	backend := findSettingArea(areas, settingAreaBackend)
	if backend == nil {
		t.Fatalf("expected backend area to exist")
	}
	if len(backend.Sections) == 0 || backend.Sections[0].Code != settingSectionLayout {
		t.Fatalf("expected backend layout section to be first, got %+v", backend.Sections)
	}

	layout := findSettingSection(backend, settingSectionLayout)
	if layout == nil {
		t.Fatalf("expected layout section to exist")
	}
	if layout.SettingCount != 1 {
		t.Fatalf("expected layout setting count = 1, got %d", layout.SettingCount)
	}
	if layout.Summary != "Swaves" {
		t.Fatalf("unexpected layout summary: %q", layout.Summary)
	}

	assets := findSettingSection(backend, settingSectionAssets)
	if assets == nil {
		t.Fatalf("expected assets section to exist")
	}
	if assets.Summary != "默认 ImageKit" {
		t.Fatalf("unexpected assets summary: %q", assets.Summary)
	}

	backup := findSettingSection(backend, settingSectionBackup)
	if backup == nil {
		t.Fatalf("expected backup section to exist")
	}
	if backup.Summary != "远程备份已开启 · S3" {
		t.Fatalf("unexpected backup summary: %q", backup.Summary)
	}

	notifications := findSettingSection(backend, settingSectionNotifications)
	if notifications == nil {
		t.Fatalf("expected notifications section to exist")
	}
	if notifications.Summary != "3 项通知已开启" {
		t.Fatalf("unexpected notifications summary: %q", notifications.Summary)
	}
}

func TestResolveSettingLocationPlacesS3UnderAssets(t *testing.T) {
	location := resolveSettingLocation(db.Setting{
		Kind: db.SettingKindThirdPartyServices,
		Code: "s3_api_endpoint",
	})
	if location.Area != settingAreaBackend || location.Section != settingSectionAssets || location.Card != "s3" {
		t.Fatalf("unexpected s3 location: %+v", location)
	}
}
