# mdeditor

**mdeditor** 是一个基于 [TinyJS](https://github.com/tarwin/tinyjsapp) 的本地桌面端 Markdown 所见即所得编辑器，独立于 swaves 主工程。

编辑器内核复用 `web/seditor`（ProseMirror），以 IIFE bundle 形式加载（`window.SEditor`），无需额外依赖。

## 技术栈

| 层 | 技术 |
|---|---|
| 框架 | [TinyJS](https://github.com/tarwin/tinyjsapp)（txiki.js 后端 + 原生 WebView） |
| 前端 | React 19 + TypeScript + Vite |
| WYSIWYG 编辑器 | `web/seditor`（ProseMirror） |
| Markdown 序列化 | seditor 内置序列化（CommonMark 兼容，与 Go 端 goldmark 一致） |

## 目录结构

```
web/mdeditor/
├── backend/
│   ├── main.ts          # TinyJS 后端：文件读写
│   └── tiny.d.ts        # TinyJS 类型定义
├── public/
│   └── seditor.js       # 构建产物（由 npm run build:seditor 生成，不提交）
├── src/
│   ├── components/
│   │   ├── Editor.tsx   # 封装 window.SEditor 的 React 组件
│   │   └── Toolbar.tsx  # 格式化工具栏（data-seditor-command 按钮）
│   ├── lib/
│   │   └── tinyapi.ts   # window.tiny API 封装
│   ├── declarations.d.ts  # window.SEditor / SEditorInstance 全局类型声明
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

### 构建 seditor bundle（首次或 seditor 更新后）

```sh
cd web/seditor && npm install   # 仅首次需要
cd web/mdeditor
npm run build:seditor           # 输出 public/seditor.js
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
npm run build      # 会自动先重新构建 seditor.js
tinyjs build
# 输出: dist/mdeditor.app (macOS)
```

## 编辑器功能

### 支持的格式

- **加粗** / *斜体* / `行内代码`
- # 标题（H1–H6）
- 无序列表 / 有序列表
- > 引用
- 代码块（带语言标注）
- 链接 / 图片

### 输入规则（Input Rules）

seditor（ProseMirror）内置：

| 输入 | 效果 |
|-----|------|
| `# ` 到 `###### ` | H1–H6 标题 |
| `* ` | 无序列表 |
| `1. ` | 有序列表 |
| `> ` | 引用块 |

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

序列化由 `web/seditor` 内部完成，基于 `prosemirror-markdown`，输出符合 **CommonMark** 规范的 Markdown。与 swaves Go 端的 `yuin/goldmark` 完全兼容。

