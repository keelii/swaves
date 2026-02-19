---
name: blog-css-modernizer
description: 结合当前博客现有视觉风格进行 CSS 设计与实现，输出简洁、易用、现代化界面。Use when users ask to redesign, polish, or implement frontend styles/components for this blog, especially for `web/templates/ui`, `web/static/style.css`, `web/static/tufte-css`, responsive behavior, accessibility, and visual consistency work.
---

# Blog CSS Modernizer

按以下顺序执行，优先满足用户明确需求，再做风格延展。

## 1) 读取现有风格上下文

先定位目标页面，再按需读取这些文件：

- `web/templates/ui/layout.html`
- `web/templates/ui/*.html` 中与目标页面相关的模板
- `web/static/style.css`（如涉及后台或全局变量）
- `web/static/tufte-css/tufte.css`（如涉及前台阅读页面）
- `web/static/oat/css/00-base.css` 和 `web/static/oat/css/01-theme.css`（如涉及 Oat 组件）

仅加载当前任务需要的文件，避免无关上下文。

## 2) 建立风格基线

在动手改代码前，提炼并记录：

- 现有色彩变量、字号层级、间距节奏、圆角和阴影风格
- 当前布局模式（阅读页、列表页、详情页、导航、评论区）
- 交互状态（hover/focus/active/disabled）是否完整
- 响应式断点与当前移动端行为

若用户只给了高层需求（例如“更现代”），默认做“渐进式焕新”，不做大改版。

## 3) 设计与实现原则

- 保留现有品牌识别：沿用已有主色与排版气质，避免无关的大幅换肤
- 优先使用 CSS 变量和现有类体系，避免硬编码重复值
- 优先做低侵入增量改造，减少模板结构变更
- 保持阅读体验稳定，不破坏正文排版与 Markdown 内容可读性
- 保证可用性：键盘可达、可见焦点、合理对比度、触屏可点按区域
- 保持简洁：控制视觉噪音，减少装饰性样式堆叠

## 4) 默认实现策略

当用户未给出细节时，使用以下默认值：

- 范围默认聚焦前台博客 UI（`web/templates/ui`），不主动改后台管理页
- 动效保持轻量（120ms-220ms），用于层级反馈，不做炫技动画
- 间距以现有尺度为基准，仅做一致性修正
- 组件状态补齐优先级高于新增视觉特效

## 5) 交付格式

每次交付都输出：

1. 变更文件与原因
2. 关键样式策略（如何兼容现有风格）
3. 响应式与可访问性覆盖说明
4. 可选后续优化项（仅列最有价值的 1-3 项）

## 6) 质量检查

提交前自检：

- 是否复用了已有变量/规则，避免无意义重复
- 是否避免了过高选择器优先级和样式污染
- 是否覆盖桌面与移动端主要路径
- 是否确保焦点样式与交互反馈存在

需要更细检查项时，读取 `references/style-audit-checklist.md`。
