package dash

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/store"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

// SettingView 用于模板展示的设置视图
type SettingView struct {
	db.Setting
	OptionsParsed  []map[string]string    // 解析后的 options（用于 radio/checkbox）
	CheckboxValues map[string]bool        // checkbox 的选中状态
	AttrsParsed    map[string]interface{} // 解析后的 attrs（用于 HTML 属性）
	Placeholder    string
	Required       bool
}

type SettingCardView struct {
	Code        string
	Label       string
	Description string
	Settings    []SettingView
}

type SettingSectionView struct {
	Code         string
	Label        string
	Description  string
	Summary      string
	SettingCount int
	Cards        []SettingCardView
}

type SettingAreaView struct {
	Code     string
	Label    string
	Sections []SettingSectionView
}

type settingLocation struct {
	Area    string
	Section string
	Card    string
}

type settingSectionMeta struct {
	Label       string
	Description string
}

type settingCardMeta struct {
	Label       string
	Description string
}

const (
	settingCodeDashNavWidth  = "dash_nav_width"
	settingCodeDashMenuState = "dash_full_main_open"
	settingCodeDashEditorTOC = "dash_post_editor_toc_open"
	settingCodeDashEditorSrc = "dash_post_editor_source_mode"
	settingCodeThemeMode     = "mode"

	settingAreaFrontend = "frontend"
	settingAreaBackend  = "backend"

	settingSectionSite          = "site"
	settingSectionAuthor        = "author"
	settingSectionContent       = "content"
	settingSectionDisplay       = "display"
	settingSectionIntegrations  = "integrations"
	settingSectionSecurity      = "security"
	settingSectionEditor        = "editor"
	settingSectionLayout        = "layout"
	settingSectionAssets        = "assets"
	settingSectionBackup        = "backup"
	settingSectionNotifications = "notifications"
	settingSectionOther         = "other"
)

var settingAreaOrder = []string{
	settingAreaFrontend,
	settingAreaBackend,
}

var settingAreaLabels = map[string]string{
	settingAreaFrontend: "前台",
	settingAreaBackend:  "后台",
}

var settingSectionOrderByArea = map[string][]string{
	settingAreaFrontend: {
		settingSectionDisplay,
		settingSectionSite,
		settingSectionAuthor,
		settingSectionContent,
		settingSectionIntegrations,
	},
	settingAreaBackend: {
		settingSectionLayout,
		settingSectionSecurity,
		settingSectionEditor,
		settingSectionAssets,
		settingSectionBackup,
		settingSectionNotifications,
		settingSectionOther,
	},
}

var settingSectionMetaByCode = map[string]settingSectionMeta{
	settingSectionSite: {
		Label:       "站点信息",
		Description: "配置公开站点的标题、访问地址、语言和页面基础信息",
	},
	settingSectionAuthor: {
		Label:       "作者信息",
		Description: "配置公开文章和站点中使用的作者身份信息",
	},
	settingSectionContent: {
		Label:       "内容链接",
		Description: "配置页面、文章、分类和标签的公开访问地址修改后可能影响既有链接",
	},
	settingSectionDisplay: {
		Label:       "用户界面",
		Description: "配置站点默认显示模式和列表阅读体验",
	},
	settingSectionIntegrations: {
		Label:       "统计与集成",
		Description: "配置公开站点的统计分析与外部集成",
	},
	settingSectionSecurity: {
		Label:       "后台访问与安全",
		Description: "配置管理后台入口地址和登录安全信息",
	},
	settingSectionEditor: {
		Label:       "编辑器",
		Description: "配置文章编辑页的默认模式和文字显示",
	},
	settingSectionLayout: {
		Label:       "用户界面",
		Description: "配置后台站点名称、导航与默认布局状态",
	},
	settingSectionAssets: {
		Label:       "资源与云服务",
		Description: "配置资源上传服务及其访问凭据",
	},
	settingSectionBackup: {
		Label:       "备份与同步",
		Description: "配置本地备份策略和远程备份目标",
	},
	settingSectionNotifications: {
		Label:       "通知与任务",
		Description: "配置站点互动和后台任务的通知策略",
	},
	settingSectionOther: {
		Label:       "其他设置",
		Description: "暂未归入标准信息结构的设置项",
	},
}

var settingCardOrderBySection = map[string][]string{
	settingSectionSite:          {"identity", "locale"},
	settingSectionAuthor:        {"profile"},
	settingSectionContent:       {"page", "post", "taxonomy"},
	settingSectionDisplay:       {"appearance", "pagination"},
	settingSectionIntegrations:  {"analytics"},
	settingSectionSecurity:      {"entry", "auth"},
	settingSectionEditor:        {"defaults", "typography"},
	settingSectionLayout:        {"identity", "navigation", "pagination"},
	settingSectionAssets:        {"provider", "see", "imagekit", "s3"},
	settingSectionBackup:        {"local", "remote"},
	settingSectionNotifications: {"interaction", "task", "policy"},
	settingSectionOther:         {"misc"},
}

var settingCardMetaBySection = map[string]map[string]settingCardMeta{
	settingSectionSite: {
		"identity": {Label: "基础信息"},
		"locale":   {Label: "语言与地区"},
	},
	settingSectionAuthor: {
		"profile": {},
	},
	settingSectionContent: {
		"page":     {Label: "全局与页面"},
		"post":     {Label: "文章链接"},
		"taxonomy": {Label: "分类与标签"},
	},
	settingSectionDisplay: {
		"appearance": {Label: "外观"},
		"pagination": {Label: "列表与分页"},
	},
	settingSectionIntegrations: {
		"analytics": {},
	},
	settingSectionSecurity: {
		"entry": {Label: "后台入口"},
		"auth":  {Label: "登录安全"},
	},
	settingSectionEditor: {
		"defaults":   {Label: "默认模式"},
		"typography": {Label: "编辑器文字"},
	},
	settingSectionLayout: {
		"identity":   {Label: "站点信息"},
		"navigation": {},
		"pagination": {Label: "列表与分页"},
	},
	settingSectionAssets: {
		"provider": {Label: "默认资源服务"},
		"see":      {Label: "S.EE"},
		"imagekit": {Label: "ImageKit"},
		"s3":       {Label: "Amazon S3"},
	},
	settingSectionBackup: {
		"local":  {Label: "本地备份"},
		"remote": {Label: "远程备份"},
	},
	settingSectionNotifications: {
		"interaction": {Label: "站点互动"},
		"task":        {Label: "系统任务"},
		"policy":      {Label: "通知策略"},
	},
	settingSectionOther: {
		"misc": {Label: "未归类"},
	},
}

var settingLocationsByCode = map[string]settingLocation{
	"site_url":                              {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	"site_name":                             {Area: settingAreaBackend, Section: settingSectionLayout, Card: "identity"},
	"site_title":                            {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	"site_desc":                             {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	"site_keywords":                         {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	"site_copyright":                        {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	db.SettingCodeBlockSearchEngineCrawlers: {Area: settingAreaFrontend, Section: settingSectionSite, Card: "identity"},
	"language":                              {Area: settingAreaFrontend, Section: settingSectionSite, Card: "locale"},
	"charset":                               {Area: settingAreaFrontend, Section: settingSectionSite, Card: "locale"},
	"timezone":                              {Area: settingAreaFrontend, Section: settingSectionSite, Card: "locale"},
	"author":                                {Area: settingAreaFrontend, Section: settingSectionAuthor, Card: "profile"},
	"author_email":                          {Area: settingAreaFrontend, Section: settingSectionAuthor, Card: "profile"},
	"base_path":                             {Area: settingAreaFrontend, Section: settingSectionContent, Card: "page"},
	"page_url_prefix":                       {Area: settingAreaFrontend, Section: settingSectionContent, Card: "page"},
	"rss_path":                              {Area: settingAreaFrontend, Section: settingSectionContent, Card: "page"},
	"post_url_prefix":                       {Area: settingAreaFrontend, Section: settingSectionContent, Card: "post"},
	"post_url_name":                         {Area: settingAreaFrontend, Section: settingSectionContent, Card: "post"},
	"post_url_ext":                          {Area: settingAreaFrontend, Section: settingSectionContent, Card: "post"},
	"category_url_prefix":                   {Area: settingAreaFrontend, Section: settingSectionContent, Card: "taxonomy"},
	"tag_url_prefix":                        {Area: settingAreaFrontend, Section: settingSectionContent, Card: "taxonomy"},
	settingCodeThemeMode:                    {Area: settingAreaFrontend, Section: settingSectionDisplay, Card: "appearance"},
	"page_size":                             {Area: settingAreaFrontend, Section: settingSectionDisplay, Card: "pagination"},
	"ga4_id":                                {Area: settingAreaFrontend, Section: settingSectionIntegrations, Card: "analytics"},
	"dash_path":                             {Area: settingAreaBackend, Section: settingSectionSecurity, Card: "entry"},
	"dash_password":                         {Area: settingAreaBackend, Section: settingSectionSecurity, Card: "auth"},
	settingCodeDashEditorTOC:                {Area: settingAreaBackend, Section: settingSectionEditor, Card: "defaults"},
	settingCodeDashEditorSrc:                {Area: settingAreaBackend, Section: settingSectionEditor, Card: "defaults"},
	"editor_font_size":                      {Area: settingAreaBackend, Section: settingSectionEditor, Card: "typography"},
	"editor_font_family":                    {Area: settingAreaBackend, Section: settingSectionEditor, Card: "typography"},
	settingCodeDashNavWidth:                 {Area: settingAreaBackend, Section: settingSectionLayout, Card: "navigation"},
	settingCodeDashMenuState:                {Area: settingAreaBackend, Section: settingSectionLayout, Card: "navigation"},
	"dash_page_size":                        {Area: settingAreaBackend, Section: settingSectionLayout, Card: "pagination"},
	"asset_default_provider":                {Area: settingAreaBackend, Section: settingSectionAssets, Card: "provider"},
	"asset_see_api_base":                    {Area: settingAreaBackend, Section: settingSectionAssets, Card: "see"},
	"asset_see_api_token":                   {Area: settingAreaBackend, Section: settingSectionAssets, Card: "see"},
	"asset_imagekit_endpoint":               {Area: settingAreaBackend, Section: settingSectionAssets, Card: "imagekit"},
	"asset_imagekit_private_key":            {Area: settingAreaBackend, Section: settingSectionAssets, Card: "imagekit"},
	"sync_push_enabled":                     {Area: settingAreaBackend, Section: settingSectionBackup, Card: "remote"},
	"sync_push_provider":                    {Area: settingAreaBackend, Section: settingSectionBackup, Card: "remote"},
	"backup_local_dir":                      {Area: settingAreaBackend, Section: settingSectionBackup, Card: "local"},
	"backup_local_interval_min":             {Area: settingAreaBackend, Section: settingSectionBackup, Card: "local"},
	"backup_local_max_count":                {Area: settingAreaBackend, Section: settingSectionBackup, Card: "local"},
	"sync_push_timeout_sec":                 {Area: settingAreaBackend, Section: settingSectionBackup, Card: "remote"},
	"s3_api_endpoint":                       {Area: settingAreaBackend, Section: settingSectionAssets, Card: "s3"},
	"s3_bucket":                             {Area: settingAreaBackend, Section: settingSectionAssets, Card: "s3"},
	"s3_access_key_id":                      {Area: settingAreaBackend, Section: settingSectionAssets, Card: "s3"},
	"s3_secret_access_key":                  {Area: settingAreaBackend, Section: settingSectionAssets, Card: "s3"},
	"notify_enable_post_like":               {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "interaction"},
	"notify_enable_comment":                 {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "interaction"},
	"notify_enable_task_success":            {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "task"},
	"notify_enable_task_error":              {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "task"},
	"notify_like_aggregate_window_min":      {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "policy"},
	"notify_retention_days":                 {Area: settingAreaBackend, Section: settingSectionNotifications, Card: "policy"},
}

var hiddenSettingCodes = map[string]struct{}{
	"font_size":       {},
	"dash_main_width": {},
}

var legacyKindLocations = map[string]settingLocation{
	db.SettingKindSiteBasics:         {Area: settingAreaFrontend, Section: settingSectionSite},
	db.SettingKindAuthorInfo:         {Area: settingAreaFrontend, Section: settingSectionAuthor},
	db.SettingKindContentRouting:     {Area: settingAreaFrontend, Section: settingSectionContent},
	db.SettingKindBackupSync:         {Area: settingAreaBackend, Section: settingSectionBackup},
	db.SettingKindThirdPartyServices: {Area: settingAreaBackend, Section: settingSectionAssets},
	db.SettingKindDashSecurity:       {Area: settingAreaBackend, Section: settingSectionSecurity},
	db.SettingKindNotifications:      {Area: settingAreaBackend, Section: settingSectionNotifications},
	db.SettingKindUIExperience:       {Area: settingAreaBackend, Section: settingSectionEditor},
}

func normalizeSettingArea(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func normalizeSettingSection(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func isHiddenSettingCode(code string) bool {
	_, hidden := hiddenSettingCodes[strings.TrimSpace(code)]
	return hidden
}

func resolveSettingLocation(s db.Setting) settingLocation {
	if isHiddenSettingCode(s.Code) {
		return settingLocation{}
	}
	if location, ok := settingLocationsByCode[strings.TrimSpace(s.Code)]; ok {
		return location
	}
	return settingLocation{
		Area:    settingAreaBackend,
		Section: settingSectionOther,
		Card:    "misc",
	}
}

func resolveLegacySettingLocation(rawKind string) settingLocation {
	return legacyKindLocations[strings.TrimSpace(rawKind)]
}

func buildSectionCards(sectionCode string) ([]SettingCardView, map[string]int) {
	order := settingCardOrderBySection[sectionCode]
	metas := settingCardMetaBySection[sectionCode]
	cards := make([]SettingCardView, 0, len(order))
	index := make(map[string]int, len(order))
	for _, cardCode := range order {
		meta := metas[cardCode]
		index[cardCode] = len(cards)
		cards = append(cards, SettingCardView{
			Code:        cardCode,
			Label:       meta.Label,
			Description: meta.Description,
			Settings:    []SettingView{},
		})
	}
	return cards, index
}

func effectiveSettingValue(setting db.Setting) string {
	if value := strings.TrimSpace(setting.Value); value != "" {
		return value
	}
	return strings.TrimSpace(setting.DefaultOptionValue)
}

func countSectionSettingViews(section SettingSectionView) int {
	total := 0
	for _, card := range section.Cards {
		total += len(card.Settings)
	}
	return total
}

func collectSectionSettingValues(section SettingSectionView) map[string]string {
	values := make(map[string]string)
	for _, card := range section.Cards {
		for _, settingView := range card.Settings {
			code := strings.TrimSpace(settingView.Code)
			if code == "" {
				continue
			}
			values[code] = effectiveSettingValue(settingView.Setting)
		}
	}
	return values
}

func joinSectionSummary(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		items = append(items, part)
	}
	return strings.Join(items, " · ")
}

func normalizeSummaryPathPart(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "/")
}

func renderSummaryPath(raw string) string {
	part := normalizeSummaryPathPart(raw)
	if part == "" {
		return "/"
	}
	return "/" + part
}

func buildPostRouteSummary(values map[string]string) string {
	segments := make([]string, 0, 2)
	if basePath := normalizeSummaryPathPart(values["base_path"]); basePath != "" {
		segments = append(segments, basePath)
	}
	if prefix := normalizeSummaryPathPart(values["post_url_prefix"]); prefix != "" {
		segments = append(segments, prefix)
	}

	name := strings.TrimSpace(values["post_url_name"])
	if name == "" {
		name = "{slug}"
	}

	ext := strings.TrimSpace(values["post_url_ext"])
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	path := "/"
	if len(segments) > 0 {
		path += strings.Join(segments, "/") + "/"
	}
	path += name + ext
	return "文章 " + path
}

func humanizeThemeModeSummary(raw string) string {
	switch normalizeThemeModeValue(raw) {
	case "dark":
		return "深色界面"
	case "light":
		return "浅色界面"
	default:
		return ""
	}
}

func humanizeAssetProviderSummary(raw string) string {
	switch normalizeAssetProvider(raw) {
	case "imagekit":
		return "ImageKit"
	case "see":
		return "S.EE"
	default:
		return ""
	}
}

func normalizeBackupProviderSummary(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "imagekit":
		return "imagekit"
	case "s3":
		return "s3"
	default:
		return ""
	}
}

func humanizeBackupProviderSummary(raw string) string {
	switch normalizeBackupProviderSummary(raw) {
	case "imagekit":
		return "ImageKit"
	case "s3":
		return "S3"
	default:
		return ""
	}
}

func isEnabledSettingValue(raw string) bool {
	value, ok := normalizeBoolSettingValue(raw)
	return ok && value == "1"
}

func buildSectionSummary(section SettingSectionView) string {
	values := collectSectionSettingValues(section)

	switch section.Code {
	case settingSectionSite:
		return joinSectionSummary(values["site_title"], values["site_url"])
	case settingSectionAuthor:
		return joinSectionSummary(values["author"], values["author_email"])
	case settingSectionContent:
		return buildPostRouteSummary(values)
	case settingSectionDisplay:
		pageSize := strings.TrimSpace(values["page_size"])
		if pageSize != "" {
			pageSize = "每页 " + pageSize + " 条"
		}
		return joinSectionSummary(humanizeThemeModeSummary(values["mode"]), pageSize)
	case settingSectionIntegrations:
		if strings.TrimSpace(values["ga4_id"]) != "" {
			return "GA4 已连接"
		}
		return "暂无统计接入"
	case settingSectionSecurity:
		passwordSummary := "未设置登录密码"
		if strings.TrimSpace(values["dash_password"]) != "" {
			passwordSummary = "已设置登录密码"
		}
		return joinSectionSummary("入口 "+renderSummaryPath(values["dash_path"]), passwordSummary)
	case settingSectionEditor:
		fontSize := strings.TrimSpace(values["editor_font_size"])
		if fontSize != "" {
			fontSize += "px"
		}
		editorMode := "默认可视模式"
		if isEnabledSettingValue(values["dash_post_editor_source_mode"]) {
			editorMode = "默认源码模式"
		}
		return joinSectionSummary(fontSize, editorMode)
	case settingSectionLayout:
		pageSize := strings.TrimSpace(values["dash_page_size"])
		if pageSize != "" {
			pageSize = "每页 " + pageSize + " 条"
		}
		navWidth := strings.TrimSpace(values["dash_nav_width"])
		if navWidth != "" {
			navWidth = "导航 " + navWidth + "px"
		}
		return joinSectionSummary(values["site_name"], navWidth, pageSize)
	case settingSectionAssets:
		provider := humanizeAssetProviderSummary(values["asset_default_provider"])
		if provider == "" {
			provider = "S.EE"
		}
		return "默认 " + provider
	case settingSectionBackup:
		if isEnabledSettingValue(values["sync_push_enabled"]) {
			provider := humanizeBackupProviderSummary(values["sync_push_provider"])
			if provider == "" {
				provider = "S3"
			}
			return "远程备份已开启 · " + provider
		}
		return "仅本地备份"
	case settingSectionNotifications:
		enabledCount := 0
		for _, code := range []string{
			"notify_enable_post_like",
			"notify_enable_comment",
			"notify_enable_task_success",
			"notify_enable_task_error",
		} {
			if isEnabledSettingValue(values[code]) {
				enabledCount++
			}
		}
		if enabledCount == 0 {
			return "通知默认关闭"
		}
		return strconv.Itoa(enabledCount) + " 项通知已开启"
	case settingSectionOther:
		if section.SettingCount > 0 {
			return strconv.Itoa(section.SettingCount) + " 项待整理设置"
		}
	}

	return ""
}

func buildSettingAreas(settings []db.Setting) []SettingAreaView {
	areas := make([]SettingAreaView, 0, len(settingAreaOrder))
	areaIndex := make(map[string]int, len(settingAreaOrder))
	sectionIndex := make(map[string]map[string]int, len(settingAreaOrder))
	cardIndex := make(map[string]map[string]map[string]int, len(settingAreaOrder))

	for _, areaCode := range settingAreaOrder {
		sections := make([]SettingSectionView, 0, len(settingSectionOrderByArea[areaCode]))
		sectionIndex[areaCode] = make(map[string]int, len(settingSectionOrderByArea[areaCode]))
		cardIndex[areaCode] = make(map[string]map[string]int, len(settingSectionOrderByArea[areaCode]))

		for _, sectionCode := range settingSectionOrderByArea[areaCode] {
			sectionMeta := settingSectionMetaByCode[sectionCode]
			cards, sectionCardIndex := buildSectionCards(sectionCode)
			sectionIndex[areaCode][sectionCode] = len(sections)
			cardIndex[areaCode][sectionCode] = sectionCardIndex
			sections = append(sections, SettingSectionView{
				Code:        sectionCode,
				Label:       sectionMeta.Label,
				Description: sectionMeta.Description,
				Cards:       cards,
			})
		}

		areaIndex[areaCode] = len(areas)
		areas = append(areas, SettingAreaView{
			Code:     areaCode,
			Label:    settingAreaLabels[areaCode],
			Sections: sections,
		})
	}

	for _, setting := range settings {
		location := resolveSettingLocation(setting)
		if location.Area == "" || location.Section == "" {
			continue
		}

		areaPos, ok := areaIndex[location.Area]
		if !ok {
			continue
		}

		sectionPos, ok := sectionIndex[location.Area][location.Section]
		if !ok {
			continue
		}

		cardCode := location.Card
		if strings.TrimSpace(cardCode) == "" {
			cardCode = "misc"
		}
		cardPos, ok := cardIndex[location.Area][location.Section][cardCode]
		if !ok {
			continue
		}

		areas[areaPos].Sections[sectionPos].Cards[cardPos].Settings = append(
			areas[areaPos].Sections[sectionPos].Cards[cardPos].Settings,
			buildSettingView(setting),
		)
	}

	filteredAreas := make([]SettingAreaView, 0, len(areas))
	for _, area := range areas {
		filteredSections := make([]SettingSectionView, 0, len(area.Sections))
		for _, section := range area.Sections {
			filteredCards := make([]SettingCardView, 0, len(section.Cards))
			for _, card := range section.Cards {
				if len(card.Settings) == 0 {
					continue
				}
				filteredCards = append(filteredCards, card)
			}
			if len(filteredCards) == 0 {
				continue
			}
			section.Cards = filteredCards
			section.SettingCount = countSectionSettingViews(section)
			section.Summary = buildSectionSummary(section)
			filteredSections = append(filteredSections, section)
		}
		if len(filteredSections) == 0 {
			continue
		}
		area.Sections = filteredSections
		filteredAreas = append(filteredAreas, area)
	}

	return filteredAreas
}

func findSettingAreaIndex(areas []SettingAreaView, areaCode string) int {
	for idx, area := range areas {
		if area.Code == areaCode {
			return idx
		}
	}
	return -1
}

func findSettingSectionIndex(area SettingAreaView, sectionCode string) int {
	for idx, section := range area.Sections {
		if section.Code == sectionCode {
			return idx
		}
	}
	return -1
}

func findSettingAreaCodeBySection(areas []SettingAreaView, sectionCode string) string {
	for _, area := range areas {
		if findSettingSectionIndex(area, sectionCode) >= 0 {
			return area.Code
		}
	}
	return ""
}

// Settings
func (h *Handler) GetSettingsHandler(c fiber.Ctx) error {
	return h.redirectToDashRoute(c, "dash.settings.all", nil, nil)
}

func (h *Handler) GetSettingsAllHandler(c fiber.Ctx) error {
	area := normalizeSettingArea(c.Query("area", ""))
	section := normalizeSettingSection(c.Query("section", ""))
	legacyKind := strings.TrimSpace(c.Query("kind", ""))
	errMsg := strings.TrimSpace(c.Query("error", ""))

	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	settingAreas := buildSettingAreas(settings)
	activeArea := SettingAreaView{}
	activeSection := SettingSectionView{}

	if len(settingAreas) > 0 {
		if area == "" && section == "" {
			legacyLocation := resolveLegacySettingLocation(legacyKind)
			area = legacyLocation.Area
			section = legacyLocation.Section
		}

		if area == "" && section != "" {
			area = findSettingAreaCodeBySection(settingAreas, section)
		}

		areaIndex := findSettingAreaIndex(settingAreas, area)
		if areaIndex < 0 {
			areaIndex = 0
		}
		activeArea = settingAreas[areaIndex]

		sectionIndex := findSettingSectionIndex(activeArea, section)
		if sectionIndex < 0 {
			sectionIndex = 0
		}
		activeSection = activeArea.Sections[sectionIndex]
	}

	return RenderDashView(c, "dash/settings_all.html", fiber.Map{
		"Title":                     "Settings - Edit All",
		"SettingAreas":              settingAreas,
		"ActiveArea":                activeArea,
		"ActiveSection":             activeSection,
		"ContentRoutingSectionCode": settingSectionContent,
		"Error":                     errMsg,
	}, "")
}

func buildSettingView(s db.Setting) SettingView {
	view := SettingView{Setting: s}

	if (s.Type == "select" || s.Type == "radio" || s.Type == "checkbox") && s.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(s.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			logger.Warn("Error parsing options for setting %s: %v", s.Options, err)
		}
	}

	if s.Type == "checkbox" {
		view.CheckboxValues = make(map[string]bool)
		if s.Value != "" {
			values := strings.Split(s.Value, ",")
			for _, v := range values {
				view.CheckboxValues[strings.TrimSpace(v)] = true
			}
		}
	}

	view.AttrsParsed = make(map[string]interface{})
	if s.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(s.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			logger.Warn("Error parsing attrs for setting %s: %v", s.Attrs, err)
		}
	}

	return view
}

type settingsValueUpdate struct {
	Code       string
	Value      string
	SkipUpdate bool
}

func isAssetSettingCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "asset_default_provider", "asset_see_api_token", "asset_imagekit_endpoint", "asset_imagekit_private_key":
		return true
	default:
		return false
	}
}

func (h *Handler) buildSettingsAllRedirectPath(c fiber.Ctx, area string, section string, errMsg string) (string, error) {
	params := map[string]string{}
	if strings.TrimSpace(area) != "" {
		params["area"] = strings.TrimSpace(area)
	}
	if strings.TrimSpace(section) != "" {
		params["section"] = strings.TrimSpace(section)
	}
	if strings.TrimSpace(errMsg) != "" {
		params["error"] = strings.TrimSpace(errMsg)
	}

	redirectPath := h.dashRouteURL(c, "dash.settings.all", nil, params)
	if redirectPath == "" {
		return "", fmt.Errorf("resolve route failed: dash.settings.all")
	}
	return redirectPath, nil
}

func (h *Handler) validateAssetSettingPayload(overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}

	values := map[string]string{
		"asset_default_provider":     strings.TrimSpace(store.GetSetting("asset_default_provider")),
		"asset_see_api_token":        strings.TrimSpace(store.GetSetting("asset_see_api_token")),
		"asset_imagekit_endpoint":    strings.TrimSpace(store.GetSetting("asset_imagekit_endpoint")),
		"asset_imagekit_private_key": strings.TrimSpace(store.GetSetting("asset_imagekit_private_key")),
	}
	hasAssetOverride := false
	for code, value := range overrides {
		if !isAssetSettingCode(code) {
			continue
		}
		values[code] = strings.TrimSpace(value)
		hasAssetOverride = true
	}
	if !hasAssetOverride {
		return nil
	}

	rawProvider := strings.TrimSpace(strings.ToLower(values["asset_default_provider"]))
	provider := normalizeAssetProvider(rawProvider)
	if rawProvider != "" && provider == "" {
		return errors.New("保存失败：资源默认服务无效，仅支持 S.EE 或 ImageKit")
	}
	if provider == "" {
		provider = "see"
	}

	switch provider {
	case "imagekit":
		if values["asset_imagekit_private_key"] == "" {
			return errors.New("保存失败：当前资源默认服务为 ImageKit，请填写 ImageKit Private Key")
		}
		if values["asset_imagekit_endpoint"] == "" {
			return errors.New("保存失败：当前资源默认服务为 ImageKit，请填写 ImageKit-endpoint")
		}
		if err := validateImageKitEndpoint(values["asset_imagekit_endpoint"]); err != nil {
			return errors.New("保存失败：" + err.Error())
		}
	case "see":
		fallthrough
	default:
		if values["asset_see_api_token"] == "" {
			return errors.New("保存失败：当前资源默认服务为 S.EE，请填写 S.EE API Token")
		}
	}

	return nil
}

func (h *Handler) PostUpdateDashUISettingAPIHandler(c fiber.Ctx) error {
	code := strings.TrimSpace(c.FormValue("code"))
	if code == "" {
		logger.Warn("[settings] update dash ui setting failed: empty code")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "设置编码不能为空",
		})
	}

	rawValue := strings.TrimSpace(c.FormValue("value"))
	if rawValue == "" {
		logger.Warn("[settings] update dash ui setting failed: empty value code=%s", code)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "设置值不能为空",
		})
	}

	switch code {
	case settingCodeDashNavWidth:
		return h.postUpdateDashNavWidthSettingAPIHandler(c, rawValue)
	case settingCodeThemeMode:
		return h.postUpdateThemeModeSettingAPIHandler(c, rawValue)
	case settingCodeDashMenuState:
		return h.postUpdateBoolSettingAPIHandler(c, code, "菜单收起状态", rawValue)
	case settingCodeDashEditorTOC:
		return h.postUpdateBoolSettingAPIHandler(c, code, "目录展开状态", rawValue)
	case settingCodeDashEditorSrc:
		return h.postUpdateBoolSettingAPIHandler(c, code, "源码模式状态", rawValue)
	default:
		logger.Warn("[settings] update dash ui setting failed: unsupported code=%s", code)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "不支持的设置项",
		})
	}
}

func (h *Handler) postUpdateDashNavWidthSettingAPIHandler(c fiber.Ctx, rawValue string) error {
	width, err := strconv.Atoi(rawValue)
	if err != nil {
		logger.Warn("[settings] update dash_nav_width failed: invalid value=%q err=%v", rawValue, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "导航栏宽度必须是整数",
		})
	}

	if width < config.DashNavWidthMin || width > config.DashNavWidthMax {
		logger.Warn("[settings] update dash_nav_width failed: out of range value=%d", width)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": fmt.Sprintf("导航栏宽度必须在 %d 到 %d 之间", config.DashNavWidthMin, config.DashNavWidthMax),
		})
	}

	if err := UpdateSettingValueService(h.Model, settingCodeDashNavWidth, strconv.Itoa(width)); err != nil {
		logger.Error("[settings] update dash_nav_width failed: value=%d err=%v", width, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "保存导航栏宽度失败，请稍后重试",
		})
	}
	if err := store.ReloadSettings(&store.GlobalStore{Model: h.Model}); err != nil {
		logger.Warn("[settings] reload settings cache failed after %s update: err=%v", settingCodeDashNavWidth, err)
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"code":  settingCodeDashNavWidth,
			"value": width,
		},
	})
}

func normalizeThemeModeValue(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "dark":
		return "dark"
	case "light":
		return "light"
	default:
		return ""
	}
}

func (h *Handler) postUpdateThemeModeSettingAPIHandler(c fiber.Ctx, rawValue string) error {
	value := normalizeThemeModeValue(rawValue)
	if value == "" {
		logger.Warn("[settings] update %s failed: invalid value=%q", settingCodeThemeMode, rawValue)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "界面模式必须是 light 或 dark",
		})
	}

	if err := UpdateSettingValueService(h.Model, settingCodeThemeMode, value); err != nil {
		logger.Error("[settings] update %s failed: value=%s err=%v", settingCodeThemeMode, value, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "保存界面模式失败，请稍后重试",
		})
	}
	if err := store.ReloadSettings(&store.GlobalStore{Model: h.Model}); err != nil {
		logger.Warn("[settings] reload settings cache failed after %s update: err=%v", settingCodeThemeMode, err)
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"code":  settingCodeThemeMode,
			"value": value,
		},
	})
}

func normalizeBoolSettingValue(raw string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "1", "true", "yes", "on":
		return "1", true
	case "0", "false", "no", "off":
		return "0", true
	default:
		return "", false
	}
}

func (h *Handler) postUpdateBoolSettingAPIHandler(c fiber.Ctx, code string, name string, rawValue string) error {
	value, ok := normalizeBoolSettingValue(rawValue)
	if !ok {
		logger.Warn("[settings] update %s failed: invalid value=%q", code, rawValue)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": name + "必须是 0 或 1",
		})
	}

	if err := UpdateSettingValueService(h.Model, code, value); err != nil {
		logger.Error("[settings] update %s failed: value=%s err=%v", code, value, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "保存" + name + "失败，请稍后重试",
		})
	}
	if err := store.ReloadSettings(&store.GlobalStore{Model: h.Model}); err != nil {
		logger.Warn("[settings] reload settings cache failed after %s update: err=%v", code, err)
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"code":  code,
			"value": value,
		},
	})
}

func (h *Handler) PostUpdateSettingsAllHandler(c fiber.Ctx) error {
	activeArea := normalizeSettingArea(c.Query("area", ""))
	activeSection := normalizeSettingSection(c.Query("section", ""))
	legacyKind := strings.TrimSpace(c.Query("kind", ""))
	if activeArea == "" && activeSection == "" && legacyKind == "" {
		return h.redirectToDashRoute(c, "dash.settings.all", nil, nil)
	}

	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	settingAreas := buildSettingAreas(settings)
	if len(settingAreas) == 0 {
		return h.redirectToDashRoute(c, "dash.settings.all", nil, nil)
	}

	if activeArea == "" && activeSection == "" {
		legacyLocation := resolveLegacySettingLocation(legacyKind)
		activeArea = legacyLocation.Area
		activeSection = legacyLocation.Section
	}
	if activeArea == "" && activeSection != "" {
		activeArea = findSettingAreaCodeBySection(settingAreas, activeSection)
	}

	areaIndex := findSettingAreaIndex(settingAreas, activeArea)
	if areaIndex < 0 {
		areaIndex = 0
	}
	activeArea = settingAreas[areaIndex].Code

	sectionIndex := findSettingSectionIndex(settingAreas[areaIndex], activeSection)
	if sectionIndex < 0 {
		sectionIndex = 0
	}
	activeSection = settingAreas[areaIndex].Sections[sectionIndex].Code

	sectionSettings := make([]db.Setting, 0)
	for _, setting := range settings {
		location := resolveSettingLocation(setting)
		if location.Area == activeArea && location.Section == activeSection {
			sectionSettings = append(sectionSettings, setting)
		}
	}
	if len(sectionSettings) == 0 {
		redirectPath, err := h.buildSettingsAllRedirectPath(c, activeArea, activeSection, "")
		if err != nil {
			return err
		}
		return webutil.RedirectTo(c, redirectPath)
	}

	updates := make([]settingsValueUpdate, 0, len(sectionSettings))
	assetOverrides := make(map[string]string)

	for _, setting := range sectionSettings {
		fieldName := "setting_" + setting.Code

		// checkbox 类型需要特殊处理，可能有多个值
		if setting.Type == "checkbox" {
			valuesBytes := c.Request().PostArgs().PeekMulti(fieldName)

			var values []string
			for _, v := range valuesBytes {
				values = append(values, string(v))
			}

			value := strings.Join(values, ",")
			updates = append(updates, settingsValueUpdate{
				Code:  setting.Code,
				Value: value,
			})
			if isAssetSettingCode(setting.Code) && value != setting.Value {
				assetOverrides[setting.Code] = value
			}
		} else {
			// 其他类型直接获取单个值
			value := c.FormValue(fieldName)
			// 对于 password 类型，如果为空则不更新（保持原值）
			if setting.Type == "password" && value == "" {
				updates = append(updates, settingsValueUpdate{
					Code:       setting.Code,
					SkipUpdate: true,
				})
				continue
			}
			updates = append(updates, settingsValueUpdate{
				Code:  setting.Code,
				Value: value,
			})
			if isAssetSettingCode(setting.Code) && value != setting.Value {
				assetOverrides[setting.Code] = value
			}
		}
	}

	if err = h.validateAssetSettingPayload(assetOverrides); err != nil {
		redirectPath, redirectErr := h.buildSettingsAllRedirectPath(c, activeArea, activeSection, err.Error())
		if redirectErr != nil {
			return redirectErr
		}
		return webutil.RedirectTo(c, redirectPath)
	}

	for _, item := range updates {
		if item.SkipUpdate {
			continue
		}
		if err = UpdateSettingValueService(h.Model, item.Code, item.Value); err != nil {
			return err
		}
	}

	redirectPath, err := h.buildSettingsAllRedirectPath(c, activeArea, activeSection, "")
	if err != nil {
		return err
	}
	return webutil.RedirectTo(c, redirectPath)
}

func (h *Handler) GetSettingEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	setting, err := GetSettingByID(h.Model, id)
	if err != nil {
		return err
	}

	// 转换为视图结构，解析 options 和 attrs
	view := SettingView{Setting: *setting}

	// 解析 options（如果是 radio、checkbox 或 select）
	if (setting.Type == "select" || setting.Type == "radio" || setting.Type == "checkbox") && setting.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(setting.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			logger.Warn("Error parsing options for setting %s: %v", setting.Options, err)
		}
	}

	// 解析 attrs（HTML 属性）
	view.AttrsParsed = make(map[string]interface{}) // 初始化为空 map，避免 nil
	if setting.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(setting.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			logger.Warn("Error parsing attrs for setting %s: %v", setting.Attrs, err)
		}
	}

	return RenderDashView(c, "dash/settings_edit.html", fiber.Map{
		"Title":   "Edit Setting",
		"Setting": view,
	}, "")
}

func (h *Handler) PostUpdateSettingHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 获取现有设置
	setting, err := GetSettingByID(h.Model, id)
	if err != nil {
		return err
	}
	originalCode := strings.TrimSpace(setting.Code)
	originalValue := setting.Value

	renderEditWithError := func(message string) error {
		view := buildSettingView(*setting)
		return RenderDashView(c, "dash/settings_edit.html", fiber.Map{
			"Title":   "Edit Setting",
			"Error":   message,
			"Setting": view,
		}, "")
	}

	// 更新字段
	setting.Kind = c.FormValue("kind")
	setting.SubKind = c.FormValue("sub_kind")
	setting.Name = c.FormValue("name")
	setting.Code = c.FormValue("code")
	setting.Type = c.FormValue("type")
	setting.Options = c.FormValue("options")
	setting.Attrs = c.FormValue("attrs")
	setting.DefaultOptionValue = c.FormValue("default_option_value")
	setting.PrefixValue = c.FormValue("prefix_value")
	setting.Description = c.FormValue("description")

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			setting.Sort = sort
		}
	}
	if c.FormValue("reload") != "" {
		setting.Reload = 1
	} else {
		setting.Reload = 0
	}

	// 处理 value 字段
	value := c.FormValue("value")
	// 对于 password 类型，如果为空则不更新（保持原值）
	if setting.Type == "password" && value == "" {
		// 保持原值，不做更新
	} else {
		setting.Value = value
	}

	assetOverrides := make(map[string]string)
	if isAssetSettingCode(originalCode) && setting.Value != originalValue {
		assetOverrides[originalCode] = setting.Value
	}
	if err = h.validateAssetSettingPayload(assetOverrides); err != nil {
		return renderEditWithError(err.Error())
	}

	if err := UpdateSettingService(h.Model, setting); err != nil {
		return renderEditWithError(err.Error())
	}

	return h.redirectToDashRoute(c, "dash.settings.list", nil, nil)
}

func (h *Handler) PostDeleteSettingHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteSettingService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.settings.list", nil, nil)
}

func (h *Handler) GetSettingNewHandler(c fiber.Ctx) error {
	// 如果需要处理预填充的 options（例如从错误返回），可以在这里解析
	var optionsParsed []map[string]string
	var defaultOptionValue string

	// 从查询参数获取 options（用于错误回显）
	if optionsJSON := c.Query("options"); optionsJSON != "" {
		if err := json.Unmarshal([]byte(optionsJSON), &optionsParsed); err == nil {
			defaultOptionValue = c.Query("default_option_value", "")
		}
	}

	return RenderDashView(c, "dash/settings_new.html", fiber.Map{
		"Title":              "New Setting",
		"OptionsParsed":      optionsParsed,
		"DefaultOptionValue": defaultOptionValue,
	}, "")
}

func (h *Handler) PostCreateSettingHandler(c fiber.Ctx) error {
	s := &db.Setting{
		Kind:               c.FormValue("kind"),
		SubKind:            c.FormValue("sub_kind"),
		Name:               c.FormValue("name"),
		Code:               c.FormValue("code"),
		Type:               c.FormValue("type"),
		Options:            c.FormValue("options"),
		Attrs:              c.FormValue("attrs"),
		Value:              c.FormValue("value"),
		DefaultOptionValue: c.FormValue("default_option_value"),
		PrefixValue:        c.FormValue("prefix_value"),
		Description:        c.FormValue("description"),
	}

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			s.Sort = sort
		}
	}
	if c.FormValue("reload") != "" {
		s.Reload = 1
	} else {
		s.Reload = 0
	}

	if err := CreateSettingService(h.Model, s); err != nil {
		// 解析 options 用于错误回显
		var optionsParsed []map[string]string
		if s.Options != "" {
			json.Unmarshal([]byte(s.Options), &optionsParsed)
		}

		return RenderDashView(c, "dash/settings_new.html", fiber.Map{
			"Title":              "New Setting",
			"Error":              err.Error(),
			"Setting":            s,
			"OptionsParsed":      optionsParsed,
			"DefaultOptionValue": s.DefaultOptionValue,
		}, "")
	}

	return h.redirectToDashRoute(c, "dash.settings.list", nil, nil)
}
