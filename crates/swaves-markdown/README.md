# swaves-markdown

`swaves-markdown` 是 swaves 的纯 Rust Markdown 渲染库，负责把 Markdown 文本转换成 HTML，并补充 front matter、目录、代码高亮、Mermaid 和数学公式渲染能力。

## 功能概览

- 解析 YAML front matter
- 生成标题锚点和目录 HTML
- 支持代码块语法高亮
- 支持 Mermaid 服务端渲染
- 支持数学公式服务端渲染
- 可在渲染失败时直接报错或保留原始源码

## 安装

```toml
[dependencies]
swaves-markdown = { path = "../crates/swaves-markdown" }
```

默认启用 `highlight`、`math` 和 `mermaid` feature；如需裁剪功能，可以关闭默认 feature 后按需开启。

## 基本使用

```rust
use swaves_markdown::{RenderOptions, render};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let source = r#"---
title: Hello
tags:
  - rust
---

# 标题

```rust
fn main() {}
```
"#;

    let result = render(source, &RenderOptions::default())?;

    assert_eq!(result.metadata["title"], "Hello");
    assert!(result.html.contains("<h1 id=\"标题\">标题</h1>"));
    assert!(result.toc_html.contains("class=\"toc\""));
    Ok(())
}
```

## 只拆分 front matter

```rust
use swaves_markdown::split;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parsed = split("---\ntitle: Demo\n---\n\n# Hello")?;
    assert_eq!(parsed.metadata["title"], "Demo");
    assert_eq!(parsed.markdown, "# Hello");
    Ok(())
}
```

## 自定义渲染选项

```rust
use swaves_markdown::{RenderFailureMode, RenderOptions, render};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let options = RenderOptions {
        generate_toc: false,
        render_failure_mode: RenderFailureMode::PreserveSource,
        ..RenderOptions::default()
    };

    let result = render("$\\definitelynotacommand{1}$", &options)?;
    assert!(result.html.contains("$\\definitelynotacommand{1}$"));
    Ok(())
}
```

## 导出的主要 API

- `split`：拆分 YAML front matter 与正文
- `render`：返回 front matter、正文 HTML 和目录 HTML
- `render_toc`：仅生成目录 HTML
- `RenderOptions`：控制目录、代码高亮、Mermaid、数学公式等行为
- `RenderFailureMode`：控制渲染失败时是报错还是保留原始源码
