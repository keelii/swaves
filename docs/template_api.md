# Swaves Frontend Template Variables

这份文档只保留前台主题模板可直接读取的顶层变量列表。

## Shared Variables

所有前台页面都会注入这些变量。

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `UrlPath` | `string` | 当前请求 path |
| `Query` | `map[string]string` | 当前请求 query |
| `IsLogin` | `bool` | 当前是否已登录后台 |
| `RouteName` | `string` | 当前路由名 |

## Page Variables

### `404.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `ReturnURL` | `string` | 返回地址 |
| `ReqID` | `string` | 请求 ID |

### `error.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `ReturnURL` | `string` | 返回地址 |
| `ReqID` | `string` | 请求 ID |

### `home.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `CanonicalURL` | `string` | canonical URL |
| `Articles` | `[]TemplatePost` | 首页文章列表 |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `Pager` | `Pagination` | 分页对象 |

### `post.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `CanonicalURL` | `string` | canonical URL |
| `MetaDescription` | `string` | meta description |
| `Post` | `TemplatePost` | 当前文章/页面 |
| `ReadUV` | `int` | 阅读 UV |
| `LikeCount` | `int` | 点赞数 |
| `Liked` | `bool` | 当前访客是否已点赞 |
| `Comments` | `[]*DisplayComment` | 评论树 |
| `CommentCount` | `int` | 评论总数 |
| `CommentPager` | `Pagination` | 评论分页 |
| `CommentFeedback` | `string` | 评论提交反馈状态 |
| `CommentForm` | `commentFormDefaults` | 评论表单回填值 |
| `CommentCaptchaRequired` | `bool` | 是否需要验证码 |
| `CommentCaptcha` | `commentCaptchaChallenge` | 评论验证码 |

### `list.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `CanonicalURL` | `string` | canonical URL |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
| `List` | `[]DisplayItem` / `[]*DisplayCategoryNode` | 列表数据 |
| `IsCategory` | `bool` | 是否为分类列表页 |

### `detail.html`

| 变量 | 类型 | 说明 |
| --- | --- | --- |
| `Title` | `string` | 页面标题 |
| `CanonicalURL` | `string` | canonical URL |
| `MetaDescription` | `string` | meta description |
| `IsCategory` | `bool` | 是否为分类详情页 |
| `IsTag` | `bool` | 是否为标签详情页 |
| `Entity` | `DisplayItem` | 当前分类/标签 |
| `List` | `[]DisplayPostRelativeInfo` | 当前分类/标签下的文章列表 |
| `ListPage` | `string` | 列表页 URL |
| `Pages` | `[]DisplayPostInfo` | 独立页面列表 |
