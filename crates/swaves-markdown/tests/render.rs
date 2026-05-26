use serde_json::json;
use swaves_markdown::{RenderFailureMode, RenderOptions, render, render_toc, split};

#[test]
fn split_extracts_front_matter_and_body() {
    let parsed = split("---\ntitle: Hello\ntags:\n  - rust\n  - markdown\n---\n\n# Body\n")
        .expect("split should succeed");

    assert_eq!(parsed.metadata.get("title"), Some(&json!("Hello")));
    assert_eq!(
        parsed.metadata.get("tags"),
        Some(&json!(["rust", "markdown"]))
    );
    assert_eq!(parsed.markdown, "# Body");
}

#[test]
fn render_generates_html_toc_and_heading_ids() {
    let result =
        render("# 标题\n\n## 第二节\n", &RenderOptions::default()).expect("render should succeed");

    assert!(
        result.html.contains("<h1 id=\"标题\">标题</h1>"),
        "{}",
        result.html
    );
    assert!(
        result.html.contains("<h2 id=\"第二节\">第二节</h2>"),
        "{}",
        result.html
    );
    assert!(
        result.toc_html.contains("class=\"toc\""),
        "{}",
        result.toc_html
    );
    assert!(
        result.toc_html.contains("href=\"#标题\""),
        "{}",
        result.toc_html
    );
}

#[test]
fn render_rewrites_standalone_images() {
    let markdown = "![logo](/logo.png \"A Logo\")\n\n![plain](/plain.png)";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(
        result
            .html
            .contains("<figure class=\"fullwidth\"><img src=\"/logo.png\" alt=\"logo\"><figcaption>A Logo</figcaption></figure>"),
        "{}",
        result.html
    );
    assert!(
        result
            .html
            .contains("<p><img src=\"/plain.png\" alt=\"plain\"></p>"),
        "{}",
        result.html
    );
}

#[test]
fn render_highlights_unlabeled_code_fences() {
    let markdown = "```\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n  fmt.Println(1)\n}\n```";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(result.html.contains("<span style="), "{}", result.html);
    assert!(result.html.contains("package main"), "{}", result.html);
}

#[test]
fn render_mermaid_server_side() {
    let markdown = "```mermaid\nflowchart LR\n    A --> B\n```";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(
        result.html.contains("data-mermaid=\"true\""),
        "{}",
        result.html
    );
    assert!(result.html.contains("<svg"), "{}", result.html);
}

#[test]
fn render_math_server_side() {
    let markdown = "$a^2 + b^2 = c^2$\n\n$$\\frac{1}{2}$$";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(
        result.html.contains("swaves-math-inline"),
        "{}",
        result.html
    );
    assert!(
        result.html.contains("swaves-math-display"),
        "{}",
        result.html
    );
    assert!(result.html.contains("<svg"), "{}", result.html);
}

#[test]
fn render_toc_returns_only_toc_html() {
    let toc = render_toc("# One\n\n## Two").expect("toc render should succeed");
    assert!(toc.toc_html.contains("class=\"toc\""), "{}", toc.toc_html);
    assert!(
        !toc.toc_html.contains("<h1 id=\"One\">"),
        "{}",
        toc.toc_html
    );
}

#[test]
fn preserve_source_on_math_error() {
    let markdown = "$\\definitelynotacommand{1}$";
    let result = render(
        markdown,
        &RenderOptions {
            render_failure_mode: RenderFailureMode::PreserveSource,
            ..RenderOptions::default()
        },
    )
    .expect("render should preserve source");

    assert!(
        result.html.contains("$\\definitelynotacommand{1}$"),
        "{}",
        result.html
    );
}

#[test]
fn render_all() {
    let markdown = include_str!("../benches/fixtures/render.md");
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");
    println!("=====");
    println!("{}", result.html);
    println!("=====");
}