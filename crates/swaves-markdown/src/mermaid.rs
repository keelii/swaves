use std::fmt;
use std::sync::atomic::{AtomicUsize, Ordering};

use ariel_rs::theme::Theme;
use comrak::adapters::CodefenceRendererAdapter;

/// Module-level counter so every mermaid diagram rendered in a process gets a
/// unique numeric suffix, even when multiple `MermaidRenderer` instances are
/// used (e.g. one per request).
static DIAGRAM_COUNTER: AtomicUsize = AtomicUsize::new(1);

pub struct MermaidRenderer;

impl MermaidRenderer {
    pub fn new() -> Self {
        Self
    }
}

impl CodefenceRendererAdapter for MermaidRenderer {
    fn write(
        &self,
        output: &mut dyn fmt::Write,
        _lang: &str,
        _meta: &str,
        code: &str,
        _sourcepos: Option<comrak::nodes::Sourcepos>,
    ) -> fmt::Result {
        let n = DIAGRAM_COUNTER.fetch_add(1, Ordering::Relaxed);
        let svg = ariel_rs::render(code.trim(), Theme::Default);
        // ariel-rs hard-codes "mermaid-svg" as the ID prefix for every element
        // (root SVG id, markers, filters, nodes, edges). Replace it with a
        // per-diagram unique prefix so multiple diagrams on the same HTML page
        // don't collide on id attributes or url(#...) marker references.
        let unique_prefix = format!("ms-{n}");
        let svg = svg.replace("mermaid-svg", &unique_prefix);
        let svg = normalize_tspan_breaks(&svg);
        output.write_str("<div class=\"mermaid\">")?;
        output.write_str(&svg)?;
        output.write_str("</div>")?;
        output.write_char('\n')
    }
}

/// ariel-rs computes node bounding boxes using a fixed single-line height
/// (`RECT_H ≈ 47.6 px`) regardless of how many lines the label contains.
/// Splitting a `<tspan>` into multiple stacked lines would cause the text to
/// overflow the pre-computed node border.
///
/// Instead this function replaces break patterns inside `<tspan>` text with a
/// space so the label stays on one line and always fits within its box.
///
/// Patterns normalised to a space:
/// - `\n`           — literal backslash-n emitted by ariel-rs from mermaid `\n` syntax
/// - `&lt;br/&gt;`  — ariel-rs HTML-escapes `<br/>` tags
/// - `&lt;br&gt;`   — same for `<br>`
fn normalize_tspan_breaks(svg: &str) -> String {
    const OPEN: &str = "<tspan>";
    const CLOSE: &str = "</tspan>";
    const BREAK_PATTERNS: &[&str] = &[
        "\\n",
        "&lt;br/&gt;",
        "&lt;br&gt;",
        "&lt;BR/&gt;",
        "&lt;BR&gt;",
    ];

    let mut result = String::with_capacity(svg.len());
    let mut remaining = svg;

    while let Some(open_pos) = remaining.find(OPEN) {
        result.push_str(&remaining[..open_pos]);
        remaining = &remaining[open_pos + OPEN.len()..];

        if let Some(close_pos) = remaining.find(CLOSE) {
            let text = &remaining[..close_pos];
            let normalized = BREAK_PATTERNS
                .iter()
                .fold(text.to_owned(), |acc, pat| acc.replace(pat, " "));

            result.push_str(OPEN);
            result.push_str(&normalized);
            result.push_str(CLOSE);
            remaining = &remaining[close_pos + CLOSE.len()..];
        } else {
            // No closing tag — pass through unchanged.
            result.push_str(OPEN);
        }
    }
    result.push_str(remaining);
    result
}

