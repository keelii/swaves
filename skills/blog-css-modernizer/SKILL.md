---
name: blog-css-modernizer
description: 博客前台与管理后台 CSS 设计与实现专家。Use when users ask to redesign, polish, or implement styles/components in this repo: (1) site frontend based on `web/static/site/tufte-css/tufte.css`, with custom styles in `web/static/site/style.css`, template files under `web/templates/site`, and Lucide icons via `web/templates/lucide_icon.html`; (2) admin backend based on Oat CSS under `web/static/dash/oat`, aligned with Oat theme tokens, with custom styles in `web/static/dash/style.css`.
---

# Blog CSS Modernizer

按以下顺序执行，先判断目标分区，再实现样式。

## 1) 判断任务属于哪个分区

先根据目标文件和页面路径选择分区：

- 前台博客：`web/templates/site/*`、`web/static/site/*`
- 管理后台：`web/templates/dash/*`、`web/static/sui/*`、`web/static/dash/*`

若同时涉及两端，分开实现并分别说明影响范围。

## 2) 前台博客规则（Site）

在前台任务中执行以下规则：

- 以 `web/static/site/tufte-css/tufte.css` 作为基础风格，不覆盖其核心阅读排版
- 优先使用原生 HTML 语义结构，不为样式目的增加多余包裹层
- 将自定义样式写入 `web/static/site/style.css`
- 保持极简风格，避免过多装饰性阴影、渐变、动画和边框
- 使用 Lucide 图标体系，复用 `web/templates/lucide_icon.html`
- 保持文章阅读体验稳定，重点关注正文、目录、评论区和分页可读性
- 补齐 hover/focus/active/disabled 状态，保持键盘可达与可见焦点

## 3) 管理后台规则（Admin）

在后台任务中执行以下规则：

- 以 Oat CSS 作为基础框架（`web/static/dash/oat/oat.min.css` 及其组件样式）
- 保持组件外观和交互与 Oat 对齐，不做明显背离的视觉风格
- 将自定义样式写入 `web/static/dash/style.css`
- 优先复用 Oat 变量与设计 token
- 颜色、间距、边框半径、阴影等规范值优先读取 `web/static/dash/oat/css/01-theme.css`（若项目改为 `00-theme.css`，以项目实际路径为准）
- 控制选择器优先级，优先局部增量覆盖，避免全局污染

## 4) 通用实现规则

- 先复用现有变量和组件，再新增自定义样式
- 先保证信息层级和可用性，再做轻量视觉优化
- 默认做渐进式改造，不做大规模重构
- 动效保持轻量（120ms-220ms），只用于反馈，不用于装饰
- 响应式优先覆盖移动端核心路径（列表、详情、表单、导航）

## 5) 交付格式

每次交付都输出：

1. 分区归属（前台/后台/两者）
2. 变更文件与原因
3. 对齐基线说明（Tufte 或 Oat）
4. 响应式与可访问性覆盖说明
5. 可选后续优化项（仅列最有价值的 1-3 项）

## 6) 质量检查

提交前自检：

- 是否只在对应分区的样式文件中落地自定义规则
- 是否复用了已有变量/规则，避免无意义重复
- 是否避免了过高选择器优先级和样式污染
- 是否覆盖桌面与移动端主要路径
- 是否确保焦点样式与交互反馈存在

需要更细检查项时，读取 `references/style-audit-checklist.md`。
