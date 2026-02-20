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
	TableComments       TableName = "t_comments"
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
	TableUniqueVisitors TableName = "t_unique_visitors"
	TableUVUnique       TableName = TableUniqueVisitors
	TableLikes          TableName = "t_likes"
	TableMedia          TableName = "t_media"
)

const InitialSQL = `
	CREATE TABLE IF NOT EXISTS ` + TablePosts + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		kind INTEGER NOT NULL DEFAULT 0,
		comment_enabled INTEGER NOT NULL DEFAULT 1,
		published_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);
	INSERT OR IGNORE INTO ` + TablePosts + ` (id, title, slug, content, status, kind, comment_enabled, published_at, created_at, updated_at) VALUES
		(1, '留言板', 'comments', '> 声音是一种机械波，而博客是一种思想波。', 'published', 1, 1, strftime('%s','now'), strftime('%s','now'), strftime('%s','now'));

	CREATE TABLE IF NOT EXISTS ` + TableComments + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER NOT NULL,
		parent_id INTEGER NOT NULL DEFAULT 0,

		author TEXT NOT NULL,
		author_email TEXT NOT NULL DEFAULT '',
		author_url TEXT NOT NULL DEFAULT '',

		author_ip TEXT NOT NULL DEFAULT '',
		visitor_id TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',

		content TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		type TEXT NOT NULL DEFAULT 'comment',

		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_comments_post_status_created
	ON ` + TableComments + ` (post_id, status, created_at ASC, id ASC);
	CREATE INDEX IF NOT EXISTS idx_comments_parent
	ON ` + TableComments + ` (parent_id, created_at ASC, id ASC);
	CREATE INDEX IF NOT EXISTS idx_comments_visitor_created
	ON ` + TableComments + ` (visitor_id, created_at DESC);

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
	-- 默认分类（初始化库时直接写入）
	INSERT OR IGNORE INTO ` + TableCategories + ` (id, parent_id, slug, name, description, sort, enabled, created_at, updated_at) VALUES
		(1, NULL, 'life', '生活', '与技术主线无直接关系的个人生活内容。', 1, 1, strftime('%s','now'), strftime('%s','now')),
		(2, 1, 'entertainment', '文娱', '音乐、电影、剧集、游戏及相关文化内容。', 2, 1, strftime('%s','now'), strftime('%s','now')),
		(3, 1, 'reading', '阅读', '读书笔记、文学随笔与阅读思考。', 3, 1, strftime('%s','now'), strftime('%s','now')),
		(4, NULL, 'work', '工作', '与职业实践、团队协作、工作方式相关内容。', 4, 1, strftime('%s','now'), strftime('%s','now')),
		(5, 4, 'career', '职业', '职业成长、管理协作、流程方法与职场经验。', 5, 1, strftime('%s','now'), strftime('%s','now')),
		(6, NULL, 'technology', '技术', '技术内容总入口，涵盖编程与软件工程实践。', 6, 1, strftime('%s','now'), strftime('%s','now')),
		(7, 6, 'programming', '编程', '代码实现、底层原理与工程技巧。', 7, 1, strftime('%s','now'), strftime('%s','now')),
		(8, 7, 'programming-languages', '编程语言', '语言特性、范式对比与生态实践。', 8, 1, strftime('%s','now'), strftime('%s','now')),
		(9, 7, 'operating-systems', '操作系统', 'Linux、macOS、Windows 与进程、内存、IO 等系统机制。', 9, 1, strftime('%s','now'), strftime('%s','now')),
		(10, 7, 'tools-productivity', '工具与效率', 'IDE、CLI、自动化与开发效率优化。', 10, 1, strftime('%s','now'), strftime('%s','now')),
		(11, 6, 'software-development', '软件开发', '从需求到上线的架构、测试、发布与维护实践。', 11, 1, strftime('%s','now'), strftime('%s','now')),
		(12, 6, 'tech-opinions', '技术观点', '技术趋势、行业观察与观点评论。', 12, 1, strftime('%s','now'), strftime('%s','now')),
		(13, NULL, 'tech', '科技', '消费科技与新品体验内容总入口。', 13, 1, strftime('%s','now'), strftime('%s','now')),
		(14, 13, 'tech-news', '发布与动态', '发布会、新品发布与科技行业动态。', 14, 1, strftime('%s','now'), strftime('%s','now')),
		(15, 13, 'product-hands-on', '产品体验', '设备开箱、上手评测与长期使用体验。', 15, 1, strftime('%s','now'), strftime('%s','now')),
		(16, 13, 'buying-guides', '选购建议', '产品对比、选购建议与购买避坑。', 16, 1, strftime('%s','now'), strftime('%s','now'));

	CREATE TABLE IF NOT EXISTS ` + TablePostCategories + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		post_id INTEGER NOT NULL,      -- ` + TablePosts + `.id
		category_id INTEGER NOT NULL,  -- ` + TableCategories + `.id
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		deleted_at INTEGER
	);
	UPDATE ` + TablePostCategories + `
	SET deleted_at = strftime('%s','now'),
		updated_at = strftime('%s','now')
	WHERE deleted_at IS NULL
	  AND id NOT IN (
		SELECT MIN(id)
		FROM ` + TablePostCategories + `
		WHERE deleted_at IS NULL
		GROUP BY post_id, category_id
	  );
	CREATE UNIQUE INDEX IF NOT EXISTS idx_post_categories_unique_active
	ON ` + TablePostCategories + ` (post_id, category_id)
	WHERE deleted_at IS NULL;

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
	INSERT OR IGNORE INTO ` + TableTasks + ` (code, name, description, schedule, enabled, kind, created_at, updated_at) VALUES
		('database_backup', '数据备份', '定时备份数据库', '@daily', 1, 0, strftime('%s','now'), strftime('%s','now'));
	INSERT OR IGNORE INTO ` + TableTasks + ` (code, name, description, schedule, enabled, kind, created_at, updated_at) VALUES
		('clear_encrypted_posts', '清理过期加密文章', '定时清理加密文章', '@every 1m', 1, 1, strftime('%s','now'), strftime('%s','now'));
	INSERT OR IGNORE INTO ` + TableTasks + ` (code, name, description, schedule, enabled, kind, created_at, updated_at) VALUES
		('remote_backup_data', '远程备份数据', '备份数据库到远程', '@daily', 1, 1, strftime('%s','now'), strftime('%s','now'));

	CREATE TABLE IF NOT EXISTS ` + TableTaskRuns + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_code TEXT NOT NULL, -- 对应 ` + TableTasks + `.code
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
		prefix_value TEXT NOT NULL DEFAULT '',
		description TEXT,
		sort INTEGER NOT NULL DEFAULT 0,
		charset TEXT,
		author TEXT,
		keywords TEXT,
		reload INTEGER NOT NULL DEFAULT 0,

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

	CREATE TABLE IF NOT EXISTS ` + TableUniqueVisitors + ` (
		entity_type INTEGER NOT NULL CHECK (entity_type IN (1, 2, 3, 4)),
		entity_id INTEGER NOT NULL DEFAULT 0,
		visitor_id BLOB NOT NULL,
		first_seen_at INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL,
		PRIMARY KEY(entity_type, entity_id, visitor_id)
	) WITHOUT ROWID;

	CREATE INDEX IF NOT EXISTS idx_unique_visitors_entity_last_seen
	ON ` + TableUniqueVisitors + ` (entity_type, entity_id, last_seen_at DESC);

	CREATE INDEX IF NOT EXISTS idx_unique_visitors_visitor_id
	ON ` + TableUniqueVisitors + ` (visitor_id);

	CREATE TABLE IF NOT EXISTS ` + TableLikes + ` (
		entity_id INTEGER NOT NULL,
		visitor_id BLOB NOT NULL,
		status INTEGER NOT NULL DEFAULT 1 CHECK (status IN (0, 1)),
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY(entity_id, visitor_id)
	) WITHOUT ROWID;

	CREATE INDEX IF NOT EXISTS idx_likes_entity_status
	ON ` + TableLikes + ` (entity_id, status);

	CREATE INDEX IF NOT EXISTS idx_likes_visitor_id
	ON ` + TableLikes + ` (visitor_id);

	CREATE TABLE IF NOT EXISTS ` + TableMedia + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,
		provider TEXT NOT NULL,
		provider_asset_id TEXT NOT NULL,
		provider_delete_key TEXT NOT NULL DEFAULT '',
		file_url TEXT NOT NULL DEFAULT '',
		original_name TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_media_provider_asset
	ON ` + TableMedia + ` (provider, provider_asset_id);

	CREATE INDEX IF NOT EXISTS idx_media_kind_created
	ON ` + TableMedia + ` (kind, created_at DESC);

	CREATE INDEX IF NOT EXISTS idx_media_provider_created
	ON ` + TableMedia + ` (provider, created_at DESC);
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
	{Sort: 10, Kind: "General", Name: "访问地址", Code: "site_url", Type: "text", Value: "http://keelii.com", Description: "站点地址，不包括路径"},
	{Sort: 11, Kind: "General", Name: "站点名称", Code: "site_name", Type: "text", Value: "Swaves", Description: "站点名称"},
	{Sort: 12, Kind: "General", Name: "站点标题", Code: "site_title", Type: "text", Value: "Nothing", Description: "站点标题"},
	{Sort: 13, Kind: "General", Name: "站点描述", Code: "site_desc", Type: "text", Value: "声音是一种机械波，而博客是一种思想波。", Description: "站点描述"},
	{Sort: 14, Kind: "General", Name: "站点关键字", Code: "keyword", Type: "text", Value: "前端开发,编程,javascript,typescript,css,html,nodejs,python,java", Description: "关键字"},
	{Sort: 15, Kind: "General", Name: "版权文字", Code: "site_copyright", Type: "text", Value: "Copyright © {{year}} keelii", Description: "站点版权"},
	{Sort: 18, Kind: "General", Name: "语言", Code: "language", Type: "select", Value: "zh-CN", Description: "语言", Options: InternalLang},
	{Sort: 19, Kind: "General", Name: "页面字符集", Code: "charset", Type: "text", Value: "utf-8", Description: "编码", Options: InternalLang},
	{Sort: 20, Kind: "General", Name: "时区", Code: "timezone", Type: "select", Value: "Asia/Shanghai", Description: "时区", Options: InternalTimezone},
	{Sort: 21, Kind: "General", Name: "管理后台密码", Code: "admin_password", Type: "password", Value: "admin", Description: "管理员密码", Attrs: `{"minlength": 6}`},
	{Sort: 22, Kind: "General", Name: "管理后台路径", Code: "admin_path", Type: "text", Value: "/admin", Description: "管理后台地址", Attrs: consts.UrlPrefixValidatorJSON},
	{Sort: 10, Kind: "Author", Name: "作者", Code: "author", Type: "text", Value: "keelii", Description: "作者"},
	{Sort: 11, Kind: "Author", Name: "邮箱", Code: "author_email", Type: "text", Value: "", Description: "作者邮箱"},
	{Sort: 12, Kind: "Author", Name: "头像", Code: "author_avatar", Type: "text", Value: "https://cdn.v2ex.com/avatar/1592/1040/7682_large.png", Description: "头像"},
	{Sort: 30, Kind: "Appearance", Name: "文字大小", Code: "font_size", Type: "range", Value: "14", Description: "UI font size", Attrs: `{"min": 12, "max": 20, "step": 2}`},
	{Sort: 31, Kind: "Appearance", Name: "界面模式", Code: "mode", Type: "radio", Value: "light", Description: "UI mode", DefaultOptionValue: "light", Options: `[{"label": "Light", "value": "light"}, {"label": "Dark", "value": "dark"}]`},
	{Sort: 32, Kind: "Appearance", Name: "Admin main width", Code: "admin_main_width", Type: "number", Value: "950", DefaultOptionValue: "950", Description: "Admin UI main width"},
	{Sort: 33, Kind: "Appearance", Name: "分页器每页数量", Code: "page_size", Type: "number", Value: "20", DefaultOptionValue: "10", Description: "每页显示的文章数量", Attrs: `{"min": 1, "max": 100}`},
	{Sort: 50, Kind: "Post", Name: "全局路径前缀", Code: "base_path", Reload: 1, Type: "prefix-field", Value: "", PrefixValue: "/", Description: "访问根路径", Attrs: consts.UrlPrefixValidatorJSON},
	{Sort: 51, Kind: "Post", Name: "页面路径前缀", Code: "page_url_prefix", Reload: 1, Type: "prefix-field", Value: "", PrefixValue: "/", Description: "页面根路径", Attrs: consts.UrlPrefixValidatorJSON},
	{Sort: 52, Kind: "Post", Name: "RSS地址", Code: "rss_path", Reload: 1, Type: "prefix-field", Value: "atom.xml", PrefixValue: "/", Description: "feed 地址", Attrs: consts.UrlFileNamePrefixValidatorJSON},
	{Sort: 53, Kind: "Post", Name: "文章地址前缀", Code: "post_url_prefix", Reload: 1, Type: "prefix-field", Value: "{datetime}", PrefixValue: "/", Attrs: consts.PostUrlPrefixValidatorJSON, Description: "文章 URL 前缀"},
	{Sort: 55, Kind: "Post", Name: "文章地址名称", Code: "post_url_name", Type: "select", Value: "{slug}", Description: "文章 URL 名称格式，注意设置成标题后其唯一性无法保证", Options: `[{"label":"URL 标识（slug）","value":"{slug}"},{"label":"文章 ID","value":"{id}"},{"label":"文章标题","value":"{title}"}]`},
	{Sort: 56, Kind: "Post", Name: "文章地址扩展名", Code: "post_url_ext", Type: "text", Value: "", Description: "分类 URL 扩展名", Attrs: consts.PostUrlExtValidatorJSON},
	{Sort: 57, Kind: "Post", Name: "分类地址前缀", Code: "category_url_prefix", Reload: 1, Type: "prefix-field", Value: "categories", PrefixValue: "/", Attrs: consts.UrlPrefixValidatorJSON, Description: "分类 URL 前缀"},
	{Sort: 58, Kind: "Post", Name: "标签地址前缀", Code: "tag_url_prefix", Reload: 1, Type: "prefix-field", Value: "tags", PrefixValue: "/", Attrs: consts.UrlPrefixValidatorJSON, Description: "标签 URL 前缀"},
	{Sort: 70, Kind: "Data", Name: "S3 远程备份开启", Code: "sync_push_enabled", Type: "radio", Value: "0", DefaultOptionValue: "0", Description: "是否启用通过 S3 API 的远程备份任务", Options: `[{"label": "关闭", "value": "0"}, {"label": "开启", "value": "1"}]`},
	{Sort: 71, Kind: "Data", Name: "本地备份目录", Code: "backup_local_dir", Type: "text", Value: "backups", Description: "本地备份目录（支持绝对路径或相对程序根目录路径）"},
	{Sort: 72, Kind: "Data", Name: "本地备份间隔 (min)", Code: "backup_local_interval_min", Type: "number", Value: "1440", DefaultOptionValue: "1440", Description: "两次本地备份之间的最小间隔（分钟）", Attrs: `{"min": 1, "max": 10080}`},
	{Sort: 73, Kind: "Data", Name: "本地备份最大数量", Code: "backup_local_max_count", Type: "number", Value: "30", DefaultOptionValue: "30", Description: "本地仅保留最新 N 个备份文件", Attrs: `{"min": 1, "max": 500}`},
	{Sort: 74, Kind: "Data", Name: "S3 API Endpoint", Code: "sync_push_endpoint", Type: "url", Value: "", Description: "S3 API Endpoint，可在 URL 路径中带 bucket（示例：https://s3.example.com/my-bucket）"},
	{Sort: 75, Kind: "Data", Name: "S3 远程备份超时 (sec)", Code: "sync_push_timeout_sec", Type: "number", Value: "60", DefaultOptionValue: "60", Description: "S3 远程备份超时时间（秒）", Attrs: `{"min": 1, "max": 600}`},
	{Sort: 90, Kind: "ThirdPart", Name: "GA4 ID", Code: "ga4_id", Type: "text", Value: "", Description: "Google Analytics 4 ID"},
	{Sort: 91, Kind: "ThirdPart", Name: "Giscus Config", Code: "giscus_config", Type: "textarea", Value: "", Description: "Giscus 配置 (JSON)"},
	{Sort: 92, Kind: "ThirdPart", Name: "媒体默认服务", Code: "media_default_provider", Type: "select", Value: "see", Description: "媒体上传默认服务商", Options: `[{"label":"S.EE","value":"see"},{"label":"ImageKit","value":"imagekit"}]`},
	{Sort: 93, Kind: "ThirdPart", Name: "S.EE API 地址", Code: "media_see_api_base", Type: "url", Value: "https://s.ee/api/v1/file/upload", Description: "S.EE API 地址（可填写上传接口完整地址）"},
	{Sort: 94, Kind: "ThirdPart", Name: "S.EE API Token", Code: "media_see_api_token", Type: "secret", Value: "", Description: "S.EE Bearer Token"},
	{Sort: 95, Kind: "ThirdPart", Name: "ImageKit-endpoint", Code: "media_imagekit_endpoint", Type: "url", Value: "https://upload.imagekit.io/api/v1", Description: "ImageKit 上传 API Endpoint"},
	{Sort: 96, Kind: "ThirdPart", Name: "ImageKit Private Key", Code: "media_imagekit_private_key", Type: "secret", Value: "", Description: "ImageKit 服务端 Private Key"},
}
