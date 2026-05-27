use serde_json::json;
use swaves_markdown::{RenderFailureMode, RenderOptions, render, render_toc, split};

const MARKDOWN_FIXTURE: &str = include_str!("fixtures/render.md");

#[test]
fn split_extracts_front_matter_and_body() {
    let document = format!(
        "---\ntitle: Hello\ntags:\n  - rust\n  - markdown\n---\n\n{}",
        MARKDOWN_FIXTURE
    );
    let parsed = split(&document).expect("split should succeed");

    assert_eq!(parsed.metadata.get("title"), Some(&json!("Hello")));
    assert_eq!(
        parsed.metadata.get("tags"),
        Some(&json!(["rust", "markdown"]))
    );
    assert_eq!(parsed.markdown.trim_end(), MARKDOWN_FIXTURE.trim_end());
}

#[test]
fn render_generates_html_toc_and_heading_ids() {
    let result = render(MARKDOWN_FIXTURE, &RenderOptions::default()).expect("render should succeed");

    assert!(
        result.html.contains("<h1 id=\"标题一\">标题一</h1>"),
        "{}",
        result.html
    );
    assert!(
        result.html.contains("<h6 id=\"标题六\">标题六</h6>"),
        "{}",
        result.html
    );
    assert!(
        result.toc_html.contains("class=\"toc\""),
        "{}",
        result.toc_html
    );
    assert!(
        result.toc_html.contains("href=\"#标题一\""),
        "{}",
        result.toc_html
    );
    assert!(
        result.html.contains("<div class=\"mermaid\">") && result.html.contains("<svg"),
        "mermaid should produce server-side SVG wrapped in .mermaid container: {}",
        result.html
    );
    assert!(
        result.html.contains("<iframe src=\"//www.slideshare.net/slideshow/embed_code/key/mchiGHfKcsWLRG\""),
        "{}",
        result.html
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
        result.html.contains("<div class=\"mermaid\">") && result.html.contains("<svg"),
        "mermaid should produce server-side SVG wrapped in .mermaid container: {}",
        result.html
    );
    assert!(
        !result.html.contains("data-mermaid=\"true\""),
        "{}",
        result.html
    );
}

#[test]
fn render_mermaid_label_newline() {
    // ariel-rs computes node boxes with a fixed single-line height, so splitting
    // a label into multiple lines would cause text to overflow the node border.
    // Instead, \n in mermaid labels is normalised to a space so the text stays
    // on one line and fits within the pre-computed box.
    let markdown = "```mermaid\nflowchart TD\n    A[\"main()\\n检查 APP_WORKER_MODE\"] --> B[End]\n```";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(
        !result.html.contains("\\n"),
        "literal \\n should have been replaced: {}",
        result.html
    );
    // No multi-line stacking — the label must stay single-line inside the box.
    assert!(
        !result.html.contains("dy=\"1.2em\""),
        "label should NOT be split into stacked tspans: {}",
        result.html
    );
    // Both parts of the label must still appear in the output.
    assert!(result.html.contains("main()"), "{}", result.html);
    assert!(result.html.contains("检查 APP_WORKER_MODE"), "{}", result.html);
}

#[test]
fn render_mermaid_label_br_tag() {
    // Same as the \n case: <br/> is normalised to a space rather than expanding
    // into stacked tspan elements that would overflow the fixed-height node box.
    let markdown = "```mermaid\nflowchart TD\n    A[\"line1<br/>line2\"] --> B[End]\n```";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    assert!(
        !result.html.contains("&lt;br"),
        "HTML-encoded <br> should have been replaced: {}",
        result.html
    );
    assert!(
        !result.html.contains("dy=\"1.2em\""),
        "label should NOT be split into stacked tspans: {}",
        result.html
    );
}

#[test]
fn render_mermaid_unique_ids_across_diagrams() {
    // When a page contains multiple mermaid diagrams, ariel-rs would normally
    // emit the same hard-coded "mermaid-svg" ID prefix for every diagram.
    // We post-process the SVG output to replace that prefix with a per-diagram
    // unique prefix so that marker/filter ids and url(#...) references from
    // different diagrams don't collide with one another.
    //
    // Note: ariel-rs itself generates some duplicate ids *within* a single
    // diagram (e.g. the edge path id appears twice). We only verify that ids
    // from *different* diagrams are distinct — intra-diagram ariel-rs quirks
    // are out of scope here.
    let markdown = "```mermaid\nflowchart LR\n    A --> B\n```\n\n```mermaid\nflowchart LR\n    C --> D\n```";
    let result = render(markdown, &RenderOptions::default()).expect("render should succeed");

    // The hard-coded ariel-rs root id must not appear at all.
    assert!(
        !result.html.contains("id=\"mermaid-svg\""),
        "hard-coded id=\"mermaid-svg\" must be replaced with a unique prefix"
    );

    // Extract the per-diagram prefix from each SVG root element id.
    // ariel-rs emits  <svg id="mermaid-svg" …>; after replacement it becomes
    // <svg id="ms-{n}" …>.  A pure numeric suffix after "ms-" identifies the
    // SVG root (as opposed to ids like "ms-1_flowchart-v2-pointEnd").
    let root_prefixes: Vec<&str> = result
        .html
        .split("id=\"ms-")
        .skip(1)
        .filter_map(|s| {
            let suffix = s.split('"').next()?;
            // Root ids are purely numeric ("1", "2", …); internal ids start
            // with "_" or "-" so they won't match here.
            if suffix.chars().all(|c| c.is_ascii_digit()) {
                Some(suffix)
            } else {
                None
            }
        })
        .collect();

    assert_eq!(
        root_prefixes.len(),
        2,
        "expected exactly two SVG root ids, got {:?}",
        root_prefixes
    );
    assert_ne!(
        root_prefixes[0], root_prefixes[1],
        "the two mermaid diagrams must receive different id prefixes"
    );

    // Confirm cross-diagram uniqueness: no id from diagram 1 appears in the
    // SVG text of diagram 2 and vice versa.
    let divs: Vec<&str> = result
        .html
        .split("<div class=\"mermaid\">")
        .skip(1)
        .collect();
    assert_eq!(divs.len(), 2, "expected two .mermaid divs");
    let prefix1 = format!("ms-{}", root_prefixes[0]);
    let prefix2 = format!("ms-{}", root_prefixes[1]);
    assert!(
        !divs[1].contains(&prefix1),
        "diagram 2 must not reference diagram 1's prefix \"{}\"",
        prefix1
    );
    assert!(
        !divs[0].contains(&prefix2),
        "diagram 1 must not reference diagram 2's prefix \"{}\"",
        prefix2
    );
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
    assert!(result.html.contains("katex-html"), "{}", result.html);
    assert!(!result.html.contains("<svg"), "{}", result.html);
}

#[test]
fn render_toc_returns_only_toc_html() {
    let toc = render_toc(MARKDOWN_FIXTURE).expect("toc render should succeed");
    assert!(toc.toc_html.contains("class=\"toc\""), "{}", toc.toc_html);
    assert!(toc.toc_html.contains("href=\"#标题一\""), "{}", toc.toc_html);
    assert!(
        !toc.toc_html.contains("<h1 id=\"标题一\">"),
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
