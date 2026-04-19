# Swaves Theme Workflow

这份文档描述当前 Swaves 前台主题的开发、运行和发布约定。

## 总体原则

- 前台主题的本地源码目录是 `web/templates/themes/<theme_code>/`
- 主题目录是扁平结构，只允许顶层 `.html` 文件
- 数据库 `themes.files` 存的是 `{filename: content}` 的 JSON
- 运行时前台模板目录最终使用一个本地目录：
  - 开发时优先直接读取 `web/templates/themes/<theme_code>/`
  - 非开发时从数据库展开到 `.cache/themes/<theme_code>/`

## 当前内置主题

- 当前内置默认主题 code 是 `tuft`
- 内置主题源码在 `web/templates/themes/tuft/`
- 应用初始化数据库时，会把二进制内嵌的 `templates/themes/tuft/` 写入 `themes.files`
- `tuft` 同时也是后台“主题”入口默认打开的主题

## 主题文件约定

当前前台主题是扁平文件结构，例如：

- `layout_main.html`
- `home.html`
- `post.html`
- `list.html`
- `detail.html`
- `404.html`
- `error.html`
- `inc_nav.html`
- `macro_content.html`

不允许：

- 子目录
- `include/nav.html` 这种嵌套路径
- 非 `.html` 模板文件

共享文件仍然保留在 `web/templates/include/`：

- `include/favicon.html`
- `include/math.html`

主题模板里可以直接：

- `{% include "include/favicon.html" %}`
- `{% include "include/math.html" %}`

## 本地开发

本地开发时，前台主题源码以 `web/templates/themes/<theme_code>/` 为准。

当前实现下：

- `TemplateReload` 开启时
- 如果当前主题 code 对应的本地目录存在
- 前台直接读取本地主题目录

这意味着你可以直接修改：

- `web/templates/themes/tuft/*.html`

然后刷新页面看效果，不需要先写回数据库。

## 数据库主题

数据库里的 `themes` 表承担两个职责：

- 保存可切换的主题记录
- 作为生产运行时主题内容来源

主题切换后：

- 当前主题记录由 `is_current=1` 标记
- 应用重启时会把该主题写到 `.cache/themes/<theme_code>/`
- 然后前台模板加载器从这个缓存目录读取主题文件

## 本地源码同步到数据库

如果你希望把本地主题源码写回数据库，使用：

```bash
go run ./cmd/theme_sync --sqlite data.sqlite --theme tuft
```

常见用法：

```bash
go run ./cmd/theme_sync --sqlite data.sqlite --theme my-theme --dir web/templates/themes/my-theme --author keelii
```

这个命令会：

- 读取本地主题目录中的顶层 `.html` 文件
- 写入数据库对应主题记录的 `files`
- 自动修正 `current_file`
- 如果主题不存在则直接创建
- 如果库里还没有当前主题，则把新建主题设为当前主题

这个命令的定位是：

- 本地主题源码 -> 数据库主题记录

不是反向同步工具。

## 推荐工作流

### 开发内置主题 `tuft`

1. 直接修改 `web/templates/themes/tuft/`
2. 本地刷新页面确认效果
3. 提交代码

如果只是发布程序本身，这一步通常不需要额外执行 `theme_sync`，因为：

- release 构建会 embed 当前仓库里的 `web/templates/themes/tuft/`
- 新初始化的数据库会自动拿到最新的内置 `tuft`

### 把主题写进已有数据库

适用于：

- 你手里已经有一个现成的 `data.sqlite`
- 你想把本地主题源码写进这个库

执行：

```bash
go run ./cmd/theme_sync --sqlite data.sqlite --theme tuft
```

### 创建自定义主题

1. 在后台创建一个新主题记录
2. 本地创建 `web/templates/themes/<theme_code>/`
3. 先在本地目录里开发和调试
4. 定稿后执行 `theme_sync` 写入数据库
5. 在后台切换当前主题并重启应用

## 运行时加载顺序

前台主题运行时加载顺序是：

1. 读取数据库当前主题 code
2. 如果是开发模式并且本地目录 `web/templates/themes/<theme_code>/` 存在，则直接读本地目录
3. 否则从数据库当前主题写入 `.cache/themes/<theme_code>/`
4. 如果当前主题不存在或展开失败，回退到内置主题 `tuft`

## 和模板 API 文档的关系

- `docs/template_api.md` 描述模板层可用的数据和函数
- 本文档描述主题源码、数据库和运行时之间的工作流
