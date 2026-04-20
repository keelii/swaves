# Swaves Frontend Template API

这份文档只保留前台主题模板可直接使用的 API 和变量，给主题作者看。

说明：

- API 名称区分大小写，文档里的 `Url` / `URL` 拼写保持与运行时完全一致。
- 这里只列 Swaves 额外注入的内容；MiniJinja 内建能力如 `safe`、`default`、`trim`、`tojson` 不重复展开。
- `Title` 已经是最终页面标题，模板里直接使用即可，不需要再二次兜底。
- `HtmlAttrs` 是推荐的属性渲染函数；`RenderAttrs` 仅为兼容旧模板保留。
- 当前运行时不会内置注入 `MetaKeywords`；如果主题需要 keywords，通常直接读取 `Settings("site_keywords")`。

## Shared Variables

所有前台页面都会注入这些顶层变量。

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 当前页面最终标题 |
| `UrlPath` | `string` | 当前请求 path |
| `Query` | `map[string]string` | 当前请求 query |
| `IsLogin` | `bool` | 当前访客是否已登录后台 |
| `RouteName` | `string` | 当前路由名 |

## Global Functions

### 常用

| 函数 | 返回值 | 说明 |
| --- | --- | --- |
| `Settings(key)` | `string` | 读取设置值 |
| `UrlFor(routeName, params?, query?)` | `string` | 按命名路由生成 URL |
| `PagerURL(page, routeName?, query?)` | `string` | 基于当前页/查询参数生成分页 URL |
| `UrlIs(routeName)` | `bool` | 当前路由是否等于目标路由 |
| `GetBasePath()` | `string` | 站点首页/基础路径 |
| `GetCategoryPrefix()` | `string` | 分类列表页路径 |
| `GetTagPrefix()` | `string` | 标签列表页路径 |
| `GetRSSUrl()` | `string` | RSS URL |
| `GetDashUrl()` | `string` | 后台首页 URL |
| `GetCategoryUrl(slug)` | `string` | 分类详情 URL |
| `GetTagUrl(slug)` | `string` | 标签详情 URL |
| `GetAvatarImage(email, author="", size=0)` | `string` | 评论头像 URL |
| `LucideIcon(name, size="16")` | `safe string` | 输出 lucide SVG 图标 |
| `_csrf_token()` | `safe string` | 输出 CSRF hidden input |

### 文本与属性辅助

| 函数 | 返回值 | 说明 |
| --- | --- | --- |
| `HtmlAttrs(map or kwargs)` | `safe string` | 渲染 HTML attributes |
| `Printf(format, ...args)` | `string` | 格式化字符串 |
| `Highlight(text, query)` | `safe string` | 输出带 `<mark>` 的高亮文本 |

### 兼容 / 少用

| 函数 | 返回值 | 说明 |
| --- | --- | --- |
| `RenderAttrs(map)` | `safe string` | 旧模板兼容用；新模板优先用 `HtmlAttrs` |
| `LongText(text, cols=30, rows=1)` | `safe string` | 输出只读 textarea |
| `GetAuthorGravatarUrl(size=0)` | `string` | 生成站点作者头像 URL |
| `GetPostUrl(post)` | `string` | 需要原始 `db.Post`；前台主题里通常直接用对象自带的 `PermLink` |

## Global Filters

这些过滤器在所有前台模板中都可直接使用。

| 过滤器 | 返回值 | 说明 |
| --- | --- | --- |
| `humanSize` | `string` | 文件大小转人类可读格式 |
| `formatTime` | `string` | Unix 时间戳转基础时间格式 |
| `relativeTime` | `string` | Unix 时间戳转相对时间 |
| `articleTime` | `string` | Unix 时间戳转文章时间格式 |
| `datetimeReplacer` | `string` | 把文本中的 `{{year}}` 替换为当前年份 |

## Page Variables

### `404.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `ReturnURL` | `string` | 返回地址 |
| `ReqID` | `string` | 请求 ID |

### `error.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `ReturnURL` | `string` | 返回地址 |
| `ReqID` | `string` | 请求 ID |

### `home.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `CanonicalURL` | `string` | canonical URL |
| `Articles` | `[]TemplatePost` | 首页文章列表 |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `Pager` | `Pagination` | 分页对象 |

### `post.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `CanonicalURL` | `string` | canonical URL |
| `MetaDescription` | `string` | meta description |
| `Post` | `TemplatePost` | 当前文章/页面 |
| `ReadUV` | `int` | 阅读 UV |
| `LikeCount` | `int` | 点赞数 |
| `Liked` | `bool` | 当前访客是否已点赞 |
| `Comments` | `[]DisplayComment` | 评论树 |
| `CommentCount` | `int` | 评论总数 |
| `CommentPager` | `Pagination` | 评论分页 |
| `CommentFeedback` | `string` | 评论状态：`approved` / `pending` / `captcha_required` / `captcha_failed` / `rate_limited` / `duplicate` |
| `CommentForm` | `CommentForm` | 评论表单回填值 |
| `CommentCaptchaRequired` | `bool` | 是否需要验证码 |
| `CommentCaptcha` | `CommentCaptcha` | 评论验证码 |

### `list.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `CanonicalURL` | `string` | canonical URL |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `List` | `[]DisplayCategoryNode` or `[]DisplayItem` | 分类列表页时为分类树；标签列表页时为标签列表 |
| `IsCategory` | `bool` | 是否为分类列表页 |

### `detail.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `CanonicalURL` | `string` | canonical URL |
| `MetaDescription` | `string` | meta description |
| `IsCategory` | `bool` | 是否为分类详情页 |
| `IsTag` | `bool` | 是否为标签详情页 |
| `Entity` | `DisplayItem` | 当前分类/标签 |
| `List` | `[]DisplayPostRelativeInfo` | 当前分类/标签下的文章列表 |
| `ListPage` | `string` | 对应列表页 URL |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |

## Common Object Shapes

这里只列模板里最常用的字段。

### `TemplatePost`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | `int64` | 文章 ID |
| `Title` | `string` | 标题 |
| `Slug` | `string` | slug |
| `Kind` | `int` | `0` 文章，`1` 页面 |
| `CommentEnabled` | `int` | `1` 表示允许评论 |
| `PublishedAt` | `int64` | 发布时间 |
| `CreatedAt` | `int64` | 创建时间 |
| `UpdatedAt` | `int64` | 更新时间 |
| `PermLink` | `string` | 永久链接 |
| `HTML` | `string` | 已渲染 HTML |
| `Prev` | `DisplayPostInfo \| nil` | 上一篇 |
| `Next` | `DisplayPostInfo \| nil` | 下一篇 |
| `Tags` | `[]DisplayItem` | 标签列表；主要在 `post.html` 可用 |
| `Category` | `DisplayItem \| nil` | 分类；主要在 `post.html` 可用 |

### `DisplayPostInfo`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | `int64` | 文章 ID |
| `Title` | `string` | 标题 |
| `Slug` | `string` | slug |
| `PermLink` | `string` | 永久链接 |
| `PublishedAt` | `int64` | 发布时间 |
| `CreatedAt` | `int64` | 创建时间 |
| `UpdatedAt` | `int64` | 更新时间 |

### `DisplayPostRelativeInfo`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | `int64` | 文章 ID |
| `Title` | `string` | 标题 |
| `Slug` | `string` | slug |
| `PermLink` | `string` | 永久链接 |
| `Tags` | `[]DisplayItem` | 标签列表 |
| `Category` | `DisplayItem \| nil` | 分类 |
| `PublishedAt` | `int64` | 发布时间 |
| `CreatedAt` | `int64` | 创建时间 |
| `UpdatedAt` | `int64` | 更新时间 |

### `DisplayItem`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | `int64` | ID |
| `Name` | `string` | 名称 |
| `Slug` | `string` | slug |
| `Description` | `string` | 描述 |
| `PermLink` | `string` | 永久链接 |
| `PostCount` | `int` | 关联文章数 |
| `CreatedAt` | `int64` | 创建时间 |
| `UpdatedAt` | `int64` | 更新时间 |

### `DisplayCategoryNode`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Item` | `DisplayItem` | 当前分类节点 |
| `Children` | `[]DisplayCategoryNode` | 子分类节点 |

### `DisplayComment`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | `int64` | 评论 ID |
| `ParentID` | `int64` | 父评论 ID |
| `Author` | `string` | 作者名 |
| `AuthorEmail` | `string` | 作者邮箱 |
| `AuthorURL` | `string` | 作者网址 |
| `Content` | `string` | 评论内容 |
| `CreatedAt` | `int64` | 创建时间 |
| `ParentAuthor` | `string` | 被回复人名称 |
| `Children` | `[]DisplayComment` | 子评论 |

### `CommentForm`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Author` | `string` | 评论昵称 |
| `AuthorEmail` | `string` | 评论邮箱 |
| `AuthorURL` | `string` | 评论网址 |
| `Content` | `string` | 评论内容 |
| `RememberMe` | `bool` | 是否记住评论者信息 |

### `CommentCaptcha`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Prompt` | `string` | 验证码题目 |
| `Token` | `string` | 验证码 token |

### `Pagination`

| 字段 / 方法 | 类型 | 说明 |
| --- | --- | --- |
| `Page` | `int` | 当前页 |
| `PageSize` | `int` | 每页数量 |
| `Num` | `int` | 总页数 |
| `Total` | `int` | 总记录数 |
| `HasPrev()` | `bool` | 是否有上一页 |
| `HasNext()` | `bool` | 是否有下一页 |
| `PrevPage()` | `int` | 上一页页码 |
| `NextPage()` | `int` | 下一页页码 |
| `GetPageItems()` | `[]PageItem` | 分页项列表 |

### `PageItem`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Type` | `string` | `number` 或 `ellipsis` |
| `Page` | `int` | 页码 |
| `Current` | `bool` | 是否当前页 |
