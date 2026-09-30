# mdeditor

**mdeditor** 是一个基于 [TinyJS](https://github.com/tarwin/tinyjsapp) 的本地桌面端 Markdown 所见即所得编辑器，独立于 swaves 主工程。

## 技术栈

| 层 | 技术 |
|---|---|
| 框架 | [TinyJS](https://github.com/tarwin/tinyjsapp)（txiki.js 后端 + 原生 WebView） |
| 前端 | React 19 + TypeScript + Vite |
| WYSIWYG 编辑器 | [TipTap](https://tiptap.dev/)（基于 ProseMirror） |
| Markdown 序列化 | [remark](https://github.com/remarkjs/remark)（remark-parse + remark-stringify） |
| 与 Go 后端兼容 | 两者均基于 **CommonMark** 规范（goldmark + micromark） |

## 目录结构

```
web/mdeditor/
├── backend/
│   ├── main.ts          # TinyJS 后端：文件读写
│   └── tiny.d.ts        # TinyJS 类型定义
├── src/
│   ├── components/
│   │   ├── Editor.tsx   # TipTap WYSIWYG 编辑器组件
│   │   └── Toolbar.tsx  # 格式化工具栏
│   ├── lib/
│   │   ├── markdown.ts  # remark 双向转换（CommonMark ↔ TipTap JSON）
│   │   └── tinyapi.ts   # window.tiny API 封装
│   ├── App.tsx
│   ├── app.css
│   └── main.tsx
├── index.html
├── package.json
├── tinyjs.json          # TinyJS 应用配置
└── vite.config.ts
```

## 安装与运行

### 前置要求

- [TinyJS](https://github.com/tarwin/tinyjsapp) 已安装（`tinyjs` 在 PATH 中）
- Node.js 18+

### 安装依赖

```sh
cd web/mdeditor
npm install
```

### 开发模式（浏览器，无文件读写）

```sh
npm run dev
```

在 `http://localhost:5173` 预览编辑器。文件打开/保存功能需要 TinyJS 运行时。

### 桌面模式（TinyJS）

```sh
cd web/mdeditor
tinyjs dev
```

### 打包为桌面应用

```sh
tinyjs build
# 输出: dist/mdeditor.app (macOS)
```

## 编辑器功能

### 支持的格式

- **加粗** / *斜体* / ~~删除线~~ / `行内代码`
- # 标题（H1–H6）
- 无序列表 / 有序列表
- > 引用
- 代码块（带语言标注）
- 分割线
- 链接 / 图片

### 输入规则（Input Rules）

TipTap StarterKit 内置：

| 输入 | 效果 |
|-----|------|
| `# ` + 空格 | H1 标题 |
| `## ` | H2 标题 |
| `### ` | H3 标题 |
| `* ` 或 `- ` | 无序列表 |
| `1. ` | 有序列表 |
| `> ` | 引用块 |
| ` ``` ` | 代码块 |

### 快捷键

| 快捷键 | 功能 |
|---|---|
| `Mod+B` | 加粗 |
| `Mod+I` | 斜体 |
| `Mod+S` | 保存 |
| `Mod+Shift+S` | 另存为 |
| `Mod+O` | 打开文件 |
| `Mod+N` | 新建 |
| `Mod+Z` | 撤销 |
| `Mod+Shift+Z` | 重做 |

## Markdown 兼容性

序列化层（`src/lib/markdown.ts`）使用 `remark-parse`（基于 micromark）解析 Markdown，使用 `remark-stringify` 输出。两者均严格遵循 **CommonMark** 规范，与 swaves Go 端的 `yuin/goldmark` 完全兼容。

输出约定（`remark-stringify` 配置）：

- 无序列表标记：`-`
- 强调标记：`_`
- 加粗标记：`*`
- 代码块围栏：`` ` ``
