# 老 Admin -> 新 SUI 功能对照表（按路由与操作粒度）

## 1. 基线与范围

- 旧 Admin（`internal/modules/admin/router.go`）命名路由：`88`
- 新 SUI（`internal/modules/sui/router.go`）路由：`9`
- 目标：用于 **全面重构（非平滑迁移）** 的功能拆解清单
- 说明：`状态=已有(演示)` 代表页面已存在但仍是 Demo/静态演示，未接完整业务数据与流程

### 当前 SUI 已有页面路由（现状）

| 路由名 | Method | Path | 模板 |
|---|---|---|---|
| `sui.home` | GET | `/sui/` | `web/templates/sui/admin_home.html` |
| `sui.posts_list` | GET | `/sui/posts_list` | `web/templates/sui/posts_list.html` |
| `sui.post_edit` | GET | `/sui/post_edit` | `web/templates/sui/post_edit.html` |
| `sui.ui.buttons` | GET | `/sui/ui/buttons` | `web/templates/sui/ui/buttons.html` |
| `sui.ui.icons` | GET | `/sui/ui/icons` | `web/templates/sui/ui/icons.html` |
| `sui.ui.forms` | GET | `/sui/ui/forms` | `web/templates/sui/ui/forms.html` |
| `sui.ui.navigation` | GET | `/sui/ui/navigation` | `web/templates/sui/ui/navigation.html` |
| `sui.ui.feedback` | GET | `/sui/ui/feedback` | `web/templates/sui/ui/feedback.html` |
| `sui.ui.data` | GET | `/sui/ui/data` | `web/templates/sui/ui/data.html` |

---

## 2. 对照表（按模块）

> 约定：新 SUI 路由名为建议命名，用于重构设计，不代表已实现。

## A. 入口、认证、工作台、开发页

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.home` | GET `/admin/` | 工作台首页 | `sui.dashboard` (`/sui/`) | 已有(演示) |
| `admin.login.show` | GET `/admin/login` | 登录页展示 | `sui.auth.login.show` (`/sui/login`) | 待实现 |
| `admin.login.submit` | POST `/admin/login` | 登录提交 | `sui.auth.login.submit` (`/sui/login`) | 待实现 |
| `admin.logout` | GET `/admin/logout` | 登出并清理会话 | `sui.auth.logout` (`/sui/logout`) | 待实现 |
| `admin.test` | GET `/admin/test` | 开发测试页 | `sui.dev.test` (`/sui/dev/test`) | 待实现(可选) |
| `admin.panic` | GET `/admin/panic` | panic 测试 | `sui.dev.panic` (`/sui/dev/panic`) | 待实现(可选) |
| `admin.dev.ui_components` | GET `/admin/dev/ui-components` | UI 组件索引页 | `sui.ui.*` (`/sui/ui/*`) | 已有(演示) |

## B. 监控与指标

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.monitor` | GET `/admin/monitor` | 监控页（图表、维度切换） | `sui.monitor` (`/sui/monitor`) | 待实现 |
| `admin.metrics.api` | GET `/admin/metrics` | 指标 API 输出 | `sui.metrics.api` (`/sui/metrics`) | 待实现 |
| `admin.monitor.data` | GET `/admin/api/monitor` | 监控数据查询 API | `sui.monitor.data` (`/sui/api/monitor`) | 待实现 |

## C. 文章（Posts）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.posts.list` | GET `/admin/posts` | 文章列表（筛选/分页） | `sui.posts.list` (`/sui/posts`) | 已有(演示, 当前为 `/sui/posts_list`) |
| `admin.posts.new` | GET `/admin/posts/new` | 新建页展示 | `sui.posts.new` (`/sui/posts/new`) | 待实现（可复用编辑页） |
| `admin.posts.create` | POST `/admin/posts/new` | 创建文章 | `sui.posts.create` (`POST /sui/posts`) | 待实现 |
| `admin.posts.edit` | GET `/admin/posts/:id/edit` | 编辑页展示 | `sui.posts.edit` (`/sui/posts/:id/edit`) | 已有(演示, 当前为 `/sui/post_edit`) |
| `admin.posts.update` | POST `/admin/posts/:id/edit` | 更新文章 | `sui.posts.update` (`POST /sui/posts/:id/edit`) | 待实现 |
| `admin.posts.delete` | POST `/admin/posts/:id/delete` | 软删除文章 | `sui.posts.delete` (`POST /sui/posts/:id/delete`) | 待实现 |

## D. 资源库（Assets）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.assets.list` | GET `/admin/assets` | 资源库页面（上传/列表/筛选） | `sui.assets.list` (`/sui/assets`) | 待实现 |
| `admin.assets.api.list` | GET `/admin/api/assets` | 资源分页查询 API | `sui.assets.api.list` (`GET /sui/api/assets`) | 待实现 |
| `admin.assets.api.upload` | POST `/admin/api/assets` | 单/多文件上传 API | `sui.assets.api.upload` (`POST /sui/api/assets`) | 待实现 |
| `admin.assets.api.batch_delete` | POST `/admin/api/assets/batch-delete` | 批量删除 API | `sui.assets.api.batch_delete` (`POST /sui/api/assets/batch-delete`) | 待实现 |
| `admin.assets.api.delete` | DELETE `/admin/api/assets/:id` | 单条删除 API | `sui.assets.api.delete` (`DELETE /sui/api/assets/:id`) | 待实现 |

## E. 评论（Comments）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.comments.list` | GET `/admin/comments` | 评论列表页 | `sui.comments.list` (`/sui/comments`) | 待实现 |
| `admin.comments.approve` | POST `/admin/comments/:id/approve` | 状态改为通过 | `sui.comments.approve` (`POST /sui/comments/:id/approve`) | 待实现 |
| `admin.comments.pending` | POST `/admin/comments/:id/pending` | 状态改为待审核 | `sui.comments.pending` (`POST /sui/comments/:id/pending`) | 待实现 |
| `admin.comments.spam` | POST `/admin/comments/:id/spam` | 状态改为垃圾 | `sui.comments.spam` (`POST /sui/comments/:id/spam`) | 待实现 |
| `admin.comments.delete` | POST `/admin/comments/:id/delete` | 删除评论 | `sui.comments.delete` (`POST /sui/comments/:id/delete`) | 待实现 |

## F. 标签（Tags）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.tags.list` | GET `/admin/tags` | 标签列表页 | `sui.tags.list` (`/sui/tags`) | 待实现 |
| `admin.tags.new` | GET `/admin/tags/new` | 新建页展示 | `sui.tags.new` (`/sui/tags/new`) | 待实现 |
| `admin.tags.create` | POST `/admin/tags/new` | 创建标签 | `sui.tags.create` (`POST /sui/tags`) | 待实现 |
| `admin.tags.edit` | GET `/admin/tags/:id/edit` | 编辑页展示 | `sui.tags.edit` (`/sui/tags/:id/edit`) | 待实现 |
| `admin.tags.update` | POST `/admin/tags/:id/edit` | 更新标签 | `sui.tags.update` (`POST /sui/tags/:id/edit`) | 待实现 |
| `admin.tags.delete` | POST `/admin/tags/:id/delete` | 删除标签 | `sui.tags.delete` (`POST /sui/tags/:id/delete`) | 待实现 |

## G. 分类（Categories）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.categories.list` | GET `/admin/categories` | 分类列表页 | `sui.categories.list` (`/sui/categories`) | 待实现 |
| `admin.categories.tree` | GET `/admin/categories/tree` | 树形分类页（拖拽/层级） | `sui.categories.tree` (`/sui/categories/tree`) | 待实现 |
| `admin.categories.parent.update` | POST `/admin/categories/:id/parent` | 更新父子关系 | `sui.categories.parent.update` (`POST /sui/categories/:id/parent`) | 待实现 |
| `admin.categories.new` | GET `/admin/categories/new` | 新建页展示 | `sui.categories.new` (`/sui/categories/new`) | 待实现 |
| `admin.categories.create` | POST `/admin/categories/new` | 创建分类 | `sui.categories.create` (`POST /sui/categories`) | 待实现 |
| `admin.categories.edit` | GET `/admin/categories/:id/edit` | 编辑页展示 | `sui.categories.edit` (`/sui/categories/:id/edit`) | 待实现 |
| `admin.categories.update` | POST `/admin/categories/:id/edit` | 更新分类 | `sui.categories.update` (`POST /sui/categories/:id/edit`) | 待实现 |
| `admin.categories.delete` | POST `/admin/categories/:id/delete` | 删除分类 | `sui.categories.delete` (`POST /sui/categories/:id/delete`) | 待实现 |

## H. 重定向（Redirects）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.redirects.list` | GET `/admin/redirects` | 重定向列表页 | `sui.redirects.list` (`/sui/redirects`) | 待实现 |
| `admin.redirects.new` | GET `/admin/redirects/new` | 新建页展示 | `sui.redirects.new` (`/sui/redirects/new`) | 待实现 |
| `admin.redirects.create` | POST `/admin/redirects/new` | 创建重定向 | `sui.redirects.create` (`POST /sui/redirects`) | 待实现 |
| `admin.redirects.edit` | GET `/admin/redirects/:id/edit` | 编辑页展示 | `sui.redirects.edit` (`/sui/redirects/:id/edit`) | 待实现 |
| `admin.redirects.update` | POST `/admin/redirects/:id/edit` | 更新重定向 | `sui.redirects.update` (`POST /sui/redirects/:id/edit`) | 待实现 |
| `admin.redirects.delete` | POST `/admin/redirects/:id/delete` | 删除重定向 | `sui.redirects.delete` (`POST /sui/redirects/:id/delete`) | 待实现 |

## I. 加密文章（Encrypted Posts）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.encrypted_posts.list` | GET `/admin/encrypted-posts` | 加密文章列表 | `sui.encrypted_posts.list` (`/sui/encrypted-posts`) | 待实现 |
| `admin.encrypted_posts.new` | GET `/admin/encrypted-posts/new` | 新建页展示 | `sui.encrypted_posts.new` (`/sui/encrypted-posts/new`) | 待实现 |
| `admin.encrypted_posts.create` | POST `/admin/encrypted-posts/new` | 创建加密文章 | `sui.encrypted_posts.create` (`POST /sui/encrypted-posts`) | 待实现 |
| `admin.encrypted_posts.edit` | GET `/admin/encrypted-posts/:id/edit` | 编辑页展示 | `sui.encrypted_posts.edit` (`/sui/encrypted-posts/:id/edit`) | 待实现 |
| `admin.encrypted_posts.update` | POST `/admin/encrypted-posts/:id/edit` | 更新加密文章 | `sui.encrypted_posts.update` (`POST /sui/encrypted-posts/:id/edit`) | 待实现 |
| `admin.encrypted_posts.delete` | POST `/admin/encrypted-posts/:id/delete` | 删除加密文章 | `sui.encrypted_posts.delete` (`POST /sui/encrypted-posts/:id/delete`) | 待实现 |

## J. 设置（Settings）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.settings.list` | GET `/admin/settings` | 设置列表页 | `sui.settings.list` (`/sui/settings`) | 待实现 |
| `admin.settings.all` | GET `/admin/settings/all` | 全量设置页 | `sui.settings.all` (`/sui/settings/all`) | 待实现 |
| `admin.settings.all.update` | POST `/admin/settings/all` | 全量设置提交 | `sui.settings.all.update` (`POST /sui/settings/all`) | 待实现 |
| `admin.settings.new` | GET `/admin/settings/new` | 新建设置页 | `sui.settings.new` (`/sui/settings/new`) | 待实现 |
| `admin.settings.create` | POST `/admin/settings/new` | 创建设置项 | `sui.settings.create` (`POST /sui/settings`) | 待实现 |
| `admin.settings.edit` | GET `/admin/settings/:id/edit` | 设置编辑页 | `sui.settings.edit` (`/sui/settings/:id/edit`) | 待实现 |
| `admin.settings.update` | POST `/admin/settings/:id/edit` | 设置更新提交 | `sui.settings.update` (`POST /sui/settings/:id/edit`) | 待实现 |
| `admin.settings.delete` | POST `/admin/settings/:id/delete` | 设置删除 | `sui.settings.delete` (`POST /sui/settings/:id/delete`) | 待实现 |

## K. 回收站（Trash）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.trash.list` | GET `/admin/trash` | 回收站总览页 | `sui.trash.list` (`/sui/trash`) | 待实现 |
| `admin.trash.posts.restore` | POST `/admin/trash/posts/:id/restore` | 恢复文章 | `sui.trash.posts.restore` (`POST /sui/trash/posts/:id/restore`) | 待实现 |
| `admin.trash.posts.delete` | POST `/admin/trash/posts/:id/delete` | 彻底删除文章 | `sui.trash.posts.delete` (`POST /sui/trash/posts/:id/delete`) | 待实现 |
| `admin.trash.encrypted_posts.restore` | POST `/admin/trash/encrypted-posts/:id/restore` | 恢复加密文章 | `sui.trash.encrypted_posts.restore` (`POST /sui/trash/encrypted-posts/:id/restore`) | 待实现 |
| `admin.trash.encrypted_posts.delete` | POST `/admin/trash/encrypted-posts/:id/delete` | 彻底删除加密文章 | `sui.trash.encrypted_posts.delete` (`POST /sui/trash/encrypted-posts/:id/delete`) | 待实现 |
| `admin.trash.tags.restore` | POST `/admin/trash/tags/:id/restore` | 恢复标签 | `sui.trash.tags.restore` (`POST /sui/trash/tags/:id/restore`) | 待实现 |
| `admin.trash.tags.delete` | POST `/admin/trash/tags/:id/delete` | 彻底删除标签 | `sui.trash.tags.delete` (`POST /sui/trash/tags/:id/delete`) | 待实现 |
| `admin.trash.categories.restore` | POST `/admin/trash/categories/:id/restore` | 恢复分类 | `sui.trash.categories.restore` (`POST /sui/trash/categories/:id/restore`) | 待实现 |
| `admin.trash.categories.delete` | POST `/admin/trash/categories/:id/delete` | 彻底删除分类 | `sui.trash.categories.delete` (`POST /sui/trash/categories/:id/delete`) | 待实现 |
| `admin.trash.redirects.restore` | POST `/admin/trash/redirects/:id/restore` | 恢复重定向 | `sui.trash.redirects.restore` (`POST /sui/trash/redirects/:id/restore`) | 待实现 |
| `admin.trash.redirects.delete` | POST `/admin/trash/redirects/:id/delete` | 彻底删除重定向 | `sui.trash.redirects.delete` (`POST /sui/trash/redirects/:id/delete`) | 待实现 |

## L. HTTP 错误日志

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.http_error_logs.list` | GET `/admin/http-error-logs` | 错误日志列表页 | `sui.http_error_logs.list` (`/sui/http-error-logs`) | 待实现 |
| `admin.http_error_logs.delete` | POST `/admin/http-error-logs/:id/delete` | 删除日志 | `sui.http_error_logs.delete` (`POST /sui/http-error-logs/:id/delete`) | 待实现 |

## M. 任务（Tasks）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.tasks.list` | GET `/admin/tasks` | 任务列表页 | `sui.tasks.list` (`/sui/tasks`) | 待实现 |
| `admin.tasks.new` | GET `/admin/tasks/new` | 新建任务页 | `sui.tasks.new` (`/sui/tasks/new`) | 待实现 |
| `admin.tasks.create` | POST `/admin/tasks/new` | 创建任务 | `sui.tasks.create` (`POST /sui/tasks`) | 待实现 |
| `admin.tasks.edit` | GET `/admin/tasks/:id/edit` | 编辑任务页 | `sui.tasks.edit` (`/sui/tasks/:id/edit`) | 待实现 |
| `admin.tasks.update` | POST `/admin/tasks/:id/edit` | 更新任务 | `sui.tasks.update` (`POST /sui/tasks/:id/edit`) | 待实现 |
| `admin.tasks.delete` | POST `/admin/tasks/:id/delete` | 删除任务 | `sui.tasks.delete` (`POST /sui/tasks/:id/delete`) | 待实现 |
| `admin.tasks.trigger` | POST `/admin/tasks/:code/trigger` | 立即触发任务 | `sui.tasks.trigger` (`POST /sui/tasks/:code/trigger`) | 待实现 |
| `admin.tasks.runs` | GET `/admin/tasks/:code/runs` | 任务运行记录页 | `sui.tasks.runs` (`/sui/tasks/:code/runs`) | 待实现 |

## N. 导入 / 导出（Import / Export）

| 旧路由名 | Method + Path | 旧操作粒度 | 新 SUI 页面/接口（建议） | 状态 |
|---|---|---|---|---|
| `admin.import.show` | GET `/admin/import` | 导入工作台页面 | `sui.import.show` (`/sui/import`) | 待实现 |
| `admin.import.submit` | POST `/admin/import` | 直接导入提交 | `sui.import.submit` (`POST /sui/import`) | 待实现 |
| `(无命名路由)` | GET `/admin/import/parse-item` | 误方法保护（返回 405） | `GET /sui/import/parse-item` (405 guard) | 待实现 |
| `admin.import.parse_item` | POST `/admin/import/parse-item` | 解析单个导入项 | `sui.import.parse_item` (`POST /sui/import/parse-item`) | 待实现 |
| `admin.import.confirm_item` | POST `/admin/import/confirm-item` | 确认导入单项 | `sui.import.confirm_item` (`POST /sui/import/confirm-item`) | 待实现 |
| `admin.import.cancel` | POST `/admin/import/cancel` | 取消导入并清理临时数据 | `sui.import.cancel` (`POST /sui/import/cancel`) | 待实现 |
| `admin.export.show` | GET `/admin/export` | 导出页面 | `sui.export.show` (`/sui/export`) | 待实现 |
| `admin.export.download` | GET `/admin/export/download` | 导出下载 | `sui.export.download` (`/sui/export/download`) | 待实现 |

---

## 3. 页面拆分建议（SUI 重构清单）

### P0 页面（先实现，支撑可用后台）

1. `sui.dashboard`
2. `sui.auth.login`
3. `sui.posts.list`
4. `sui.posts.edit`（含新建）
5. `sui.assets.list`
6. `sui.settings.all`
7. `sui.import.show`
8. `sui.export.show`

### P1 页面（业务完整性）

1. `sui.comments.list`
2. `sui.tags.*`
3. `sui.categories.*`（含 tree 与 parent update）
4. `sui.redirects.*`
5. `sui.tasks.*`
6. `sui.trash.list`

### P2 页面（运维与高级能力）

1. `sui.monitor` + `sui.monitor.data` + `sui.metrics.api`
2. `sui.encrypted_posts.*`
3. `sui.http_error_logs.*`
4. `sui.dev.test` / `sui.dev.panic`（可选）

---

## 4. 最小验收口径（按操作粒度）

- 每个列表页至少覆盖：筛选、分页、批量/单条操作入口
- 每个编辑页至少覆盖：加载、校验失败回显、提交成功跳转
- 每个 destructive 操作至少覆盖：确认、错误提示、成功反馈
- 每个 API 至少覆盖：鉴权、CSRF、错误返回结构一致
- 导入流程必须覆盖：`parse -> edit -> confirm` 与 `cancel-cleanup`

