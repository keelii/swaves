package db

import "swaves/internal/consts"

type TableOp string

const (
	TableOpInsert TableOp = "insert"
	TableOpUpdate TableOp = "update"
	TableOpDelete TableOp = "delete"
)
const (
	TableSessions       TableName = "t_admin_sessions"
	TablePosts          TableName = "t_posts"
	TableEncryptedPosts TableName = "t_encrypted_posts"
	TableTags           TableName = "t_tags"
	TableRedirects      TableName = "t_redirects"
	TableSettings       TableName = "t_settings"
	TableTasks          TableName = "t_tasks"
	TableCategories     TableName = "t_categories"
	TablePostTags       TableName = "t_post_tags"
	TablePostCategories TableName = "t_post_categories"
	TableTaskRuns       TableName = "t_task_runs"
	TableHttpErrorLogs  TableName = "t_http_error_logs"
)

const InitialSQL = `
	CREATE TABLE IF NOT EXISTS ` + TablePosts + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		kind INTEGER NOT NULL DEFAULT 0,
		published_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TableEncryptedPosts + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		content TEXT NOT NULL,
		password TEXT NOT NULL,
		expires_at INTEGER,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TableCategories + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		parent_id INTEGER,                -- 父分类，NULL 表示顶级分类
		slug TEXT NOT NULL DEFAULT '',    -- 访问路径

		name TEXT NOT NULL,                -- 展示名称
		description TEXT NOT NULL DEFAULT '',

		sort INTEGER NOT NULL DEFAULT 0,   -- 同级排序
		enabled INTEGER NOT NULL DEFAULT 1, -- 1=启用 0=禁用

		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);
	CREATE TABLE IF NOT EXISTS ` + TablePostCategories + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		post_id INTEGER NOT NULL,      -- ` + TablePosts + `.id
		category_id INTEGER NOT NULL,  -- ` + TableCategories + `.id
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TableTags + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TablePostTags + ` (
		post_id INTEGER NOT NULL,
		tag_id INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER,
		UNIQUE(post_id, tag_id)
	);

	CREATE TABLE IF NOT EXISTS ` + TableRedirects + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_path TEXT NOT NULL UNIQUE,
		to_path TEXT NOT NULL,
		status INTEGER NOT NULL DEFAULT 301,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TableTasks + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE, --任务唯一标识，必须唯一
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		schedule TEXT NOT NULL, -- cron 表达式，如 "0 */5 * * *"
		enabled INTEGER NOT NULL DEFAULT 1,
		kind INTEGER NOT NULL DEFAULT 0, -- 任务类型：0=Internal(不生成TaskRun) 
		last_run_at INTEGER,
		last_status TEXT, -- 最后一次执行状态: "pending", "success", "error"
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);
	DELETE FROM ` + TableTasks + ` WHERE code = 'database_backup';
    INSERT INTO ` + TableTasks + ` (code, name, description, schedule, enabled, kind, created_at, updated_at) VALUES
		('database_backup', '数据备份', '定备份置数据库', '@daily', 1, 0, strftime('%s','now'), strftime('%s','now'));
	DELETE FROM ` + TableTasks + ` WHERE code = 'clear_encrypted_posts';
    INSERT INTO ` + TableTasks + ` (code, name, description, schedule, enabled, kind, created_at, updated_at) VALUES
		('clear_encrypted_posts', '清理过期加密文章', '定时清理加密文章', '@every 1m', 1, 0, strftime('%s','now'), strftime('%s','now'));

	CREATE TABLE IF NOT EXISTS ` + TableTaskRuns + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_code TEXT NOT NULL, -- 对应 ` + TableTasks + `.code
		run_id TEXT NOT NULL, -- 本次执行唯一标识 UUID
		status TEXT NOT NULL, -- "pending", "success" 或 "error"
		message TEXT NOT NULL DEFAULT '',
		started_at INTEGER NOT NULL,
		finished_at INTEGER NOT NULL,
		duration INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS ` + TableSettings + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		kind TEXT NOT NULL DEFAULT 'default',
		name TEXT NOT NULL,
		code TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		options TEXT,
		attrs TEXT,
		value TEXT,
		default_option_value TEXT,
		description TEXT,
		sort INTEGER NOT NULL DEFAULT 0,
		charset TEXT,
		author TEXT,
		keywords TEXT,

		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS ` + TableHttpErrorLogs + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		req_id TEXT NOT NULL,
		client_ip TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		status INTEGER NOT NULL,
		user_agent TEXT NOT NULL,

		query_params TEXT,
		body_params TEXT,

		created_at INTEGER NOT NULL,
		expired_at INTEGER NOT NULL
	);
`
const InternalLang = `[
	  {"label": "简体中文（中国大陆）", "value": "zh-CN"},
	  {"label": "简体中文（新加坡）", "value": "zh-SG"},
	  {"label": "简体中文", "value": "zh-Hans"},
	  {"label": "简体中文（中国大陆）", "value": "zh-Hans-CN"},
	  {"label": "繁体中文（台湾）", "value": "zh-TW"},
	  {"label": "繁体中文（香港）", "value": "zh-HK"},
	  {"label": "繁体中文（澳门）", "value": "zh-MO"},
	  {"label": "繁体中文", "value": "zh-Hant"},
	  {"label": "繁体中文（台湾）", "value": "zh-Hant-TW"},
	  {"label": "繁体中文（香港）", "value": "zh-Hant-HK"},
	  {"label": "中文", "value": "zh"},
	  {"label": "英语（美国）", "value": "en-US"},
	  {"label": "英语（英国）", "value": "en-GB"},
	  {"label": "英语（加拿大）", "value": "en-CA"},
	  {"label": "英语（澳大利亚）", "value": "en-AU"},
	  {"label": "英语（印度）", "value": "en-IN"},
	  {"label": "英语", "value": "en"},
	  {"label": "日语（日本）", "value": "ja-JP"},
	  {"label": "日语", "value": "ja"},
	  {"label": "韩语（韩国）", "value": "ko-KR"},
	  {"label": "韩语", "value": "ko"},
	  {"label": "法语（法国）", "value": "fr-FR"},
	  {"label": "法语（加拿大）", "value": "fr-CA"},
	  {"label": "法语", "value": "fr"},
	  {"label": "德语（德国）", "value": "de-DE"},
	  {"label": "德语（奥地利）", "value": "de-AT"},
	  {"label": "德语", "value": "de"},
	  {"label": "西班牙语（西班牙）", "value": "es-ES"},
	  {"label": "西班牙语（墨西哥）", "value": "es-MX"},
	  {"label": "西班牙语（美国）", "value": "es-US"},
	  {"label": "西班牙语", "value": "es"},
	  {"label": "俄语（俄罗斯）", "value": "ru-RU"},
	  {"label": "俄语", "value": "ru"},
	  {"label": "葡萄牙语（葡萄牙）", "value": "pt-PT"},
	  {"label": "葡萄牙语（巴西）", "value": "pt-BR"},
	  {"label": "葡萄牙语", "value": "pt"},
	  {"label": "阿拉伯语（沙特阿拉伯）", "value": "ar-SA"},
	  {"label": "阿拉伯语（埃及）", "value": "ar-EG"},
	  {"label": "阿拉伯语", "value": "ar"},
	  {"label": "意大利语（意大利）", "value": "it-IT"},
	  {"label": "意大利语", "value": "it"},
	  {"label": "荷兰语（荷兰）", "value": "nl-NL"},
	  {"label": "荷兰语（比利时）", "value": "nl-BE"},
	  {"label": "荷兰语", "value": "nl"},
	  {"label": "土耳其语（土耳其）", "value": "tr-TR"},
	  {"label": "土耳其语", "value": "tr"},
	  {"label": "越南语（越南）", "value": "vi-VN"},
	  {"label": "越南语", "value": "vi"},
	  {"label": "泰语（泰国）", "value": "th-TH"},
	  {"label": "泰语", "value": "th"},
	  {"label": "印地语（印度）", "value": "hi-IN"},
	  {"label": "印地语", "value": "hi"}
	]`

const InternalTimezone = `[
	  {"label": "中国标准时间 (北京)", "value": "Asia/Shanghai"},
	  {"label": "中国标准时间 (乌鲁木齐)", "value": "Asia/Urumqi"},
	  {"label": "香港时间", "value": "Asia/Hong_Kong"},
	  {"label": "台北时间", "value": "Asia/Taipei"},
	  {"label": "澳门时间", "value": "Asia/Macau"},
	  {"label": "美国东部时间 (纽约)", "value": "America/New_York"},
	  {"label": "美国中部时间 (芝加哥)", "value": "America/Chicago"},
	  {"label": "美国山区时间 (丹佛)", "value": "America/Denver"},
	  {"label": "美国太平洋时间 (洛杉矶)", "value": "America/Los_Angeles"},
	  {"label": "美国阿拉斯加时间 (安克雷奇)", "value": "America/Anchorage"},
	  {"label": "美国夏威夷时间 (檀香山)", "value": "Pacific/Honolulu"},
	  {"label": "英国时间 (伦敦)", "value": "Europe/London"},
	  {"label": "欧洲中部时间 (巴黎/柏林)", "value": "Europe/Paris"},
	  {"label": "东欧时间 (莫斯科)", "value": "Europe/Moscow"},
	  {"label": "中东时间 (迪拜)", "value": "Asia/Dubai"},
	  {"label": "印度标准时间 (新德里)", "value": "Asia/Kolkata"},
	  {"label": "日本标准时间 (东京)", "value": "Asia/Tokyo"},
	  {"label": "韩国标准时间 (首尔)", "value": "Asia/Seoul"},
	  {"label": "澳大利亚东部时间 (悉尼)", "value": "Australia/Sydney"},
	  {"label": "澳大利亚中部时间 (阿德莱德)", "value": "Australia/Adelaide"},
	  {"label": "澳大利亚西部时间 (珀斯)", "value": "Australia/Perth"},
	  {"label": "新西兰时间 (奥克兰)", "value": "Pacific/Auckland"},
	  {"label": "新加坡时间", "value": "Asia/Singapore"},
	  {"label": "马来西亚时间 (吉隆坡)", "value": "Asia/Kuala_Lumpur"},
	  {"label": "泰国时间 (曼谷)", "value": "Asia/Bangkok"},
	  {"label": "越南时间 (河内)", "value": "Asia/Ho_Chi_Minh"},
	  {"label": "印度尼西亚西部时间 (雅加达)", "value": "Asia/Jakarta"},
	  {"label": "印度尼西亚中部时间 (巴厘岛)", "value": "Asia/Makassar"},
	  {"label": "印度尼西亚东部时间 (查亚普拉)", "value": "Asia/Jayapura"},
	  {"label": "菲律宾时间 (马尼拉)", "value": "Asia/Manila"},
	  {"label": "加拿大东部时间 (多伦多)", "value": "America/Toronto"},
	  {"label": "加拿大中部时间 (温尼伯)", "value": "America/Winnipeg"},
	  {"label": "加拿大山地时间 (埃德蒙顿)", "value": "America/Edmonton"},
	  {"label": "加拿大太平洋时间 (温哥华)", "value": "America/Vancouver"},
	  {"label": "巴西东部时间 (圣保罗)", "value": "America/Sao_Paulo"},
	  {"label": "巴西西部时间 (马瑙斯)", "value": "America/Manaus"},
	  {"label": "阿根廷时间 (布宜诺斯艾利斯)", "value": "America/Argentina/Buenos_Aires"},
	  {"label": "墨西哥时间 (墨西哥城)", "value": "America/Mexico_City"},
	  {"label": "南非时间 (约翰内斯堡)", "value": "Africa/Johannesburg"},
	  {"label": "埃及时间 (开罗)", "value": "Africa/Cairo"},
	  {"label": "沙特阿拉伯时间 (利雅得)", "value": "Asia/Riyadh"},
	  {"label": "以色列时间 (耶路撒冷)", "value": "Asia/Jerusalem"},
	  {"label": "土耳其时间 (伊斯坦布尔)", "value": "Europe/Istanbul"},
	  {"label": "协调世界时 (UTC)", "value": "UTC"},
	  {"label": "格林威治标准时间", "value": "GMT"}
]`

var DefaultSettings = []Setting{
	{Sort: 1, Kind: "General", Name: "Site URL", Code: "site_url", Type: "text", Value: "http://keelii.com", Description: "站点地址，不包括路径"},
	{Sort: 2, Kind: "General", Name: "Site Name", Code: "site_name", Type: "text", Value: "swaves", Description: "站点名称"},
	{Sort: 2, Kind: "General", Name: "Site Description", Code: "site_desc", Type: "text", Value: "Nothing", Description: "站点描述"},
	{Sort: 2, Kind: "General", Name: "Site Copyright", Code: "site_copyright", Type: "text", Value: "Copyright © {{year}}", Description: "站点版权"},
	{Sort: 4, Kind: "General", Name: "Author", Code: "author", Type: "text", Value: "keelii", Description: "作者"},
	{Sort: 5, Kind: "General", Name: "Keywords", Code: "keyword", Type: "text", Value: "前端开发,编程,javascript,typescript,css,html,nodejs,python,java", Description: "关键字"},
	{Sort: 6, Kind: "General", Name: "Language", Code: "language", Type: "select", Value: "zh-CN", Description: "语言", Options: InternalLang},
	{Sort: 7, Kind: "General", Name: "Charset", Code: "charset", Type: "text", Value: "utf-8", Description: "编码", Options: InternalLang},
	{Sort: 9, Kind: "General", Name: "Timezone", Code: "timezone", Type: "select", Value: "Asia/Shanghai", Description: "时区", Options: InternalTimezone},
	{Sort: 11, Kind: "General", Name: "Admin Password", Code: "admin_password", Type: "password", Value: "admin", Description: "管理员密码", Attrs: `{"minlength": 6}`},
	{Sort: 11, Kind: "Appearance", Name: "Font size", Code: "font_size", Type: "range", Value: "14", Description: "UI font size", Attrs: `{"min": 12, "max": 20, "step": 2}`},
	{Sort: 11, Kind: "Appearance", Name: "Mode", Code: "mode", Type: "radio", Value: "light", Description: "UI mode", DefaultOptionValue: "light", Options: `[{"label": "Light", "value": "light"}, {"label": "Dark", "value": "dark"}]`},
	{Sort: 11, Kind: "Appearance", Name: "Admin main width", Code: "admin_main_width", Type: "number", Value: "950", DefaultOptionValue: "950", Description: "Admin UI main width"},
	{Sort: 11, Kind: "Appearance", Name: "Page size", Code: "page_size", Type: "number", Value: "10", DefaultOptionValue: "10", Description: "每页显示的文章数量", Attrs: `{"min": 1, "max": 100}`},
	{Sort: 11, Kind: "Post", Name: "Base Path", Code: "base_path", Type: "text", Value: "/", Description: "访问根路径", Attrs: consts.UrlPrefixValidatorJSON},
	{Sort: 13, Kind: "Post", Name: "Page Path", Code: "page_path", Type: "text", Value: "/", Description: "页面根路径", Attrs: consts.UrlPrefixValidatorJSON},
	{Sort: 13, Kind: "Post", Name: "RSS Url", Code: "rss_path", Type: "text", Value: "/atom.xml", Description: "feed 地址", Attrs: consts.UrlFileNamePrefixValidatorJSON},
	{Sort: 13, Kind: "Post", Name: "Post Url Prefix", Code: "post_url_prefix", Type: "text", Value: "/{datetime}", Attrs: consts.PostUrlPrefixValidatorJSON, Description: "文章 URL 前缀"},
	{Sort: 15, Kind: "Post", Name: "Tag Url Prefix", Code: "tag_url_prefix", Type: "text", Value: "/tags", Attrs: consts.UrlPrefixValidatorJSON, Description: "标签 URL 前缀"},
	{Sort: 17, Kind: "Post", Name: "Category Index", Code: "category_index", Type: "text", Value: "/categories", Attrs: consts.UrlPrefixValidatorJSON, Description: "分类页面地址"},
	{Sort: 19, Kind: "ThirdPart", Name: "GA4 ID", Code: "ga4_id", Type: "text", Value: "", Description: "Google Analytics 4 ID"},
	{Sort: 21, Kind: "ThirdPart", Name: "Giscus Config", Code: "giscus_config", Type: "textarea", Value: "", Description: "Giscus 配置 (JSON)"},
}
