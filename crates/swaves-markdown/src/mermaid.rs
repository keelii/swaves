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
        let svg = fix_tspan_newlines(&svg);
        output.write_str("<div class=\"mermaid\">")?;
        output.write_str(&svg)?;
        output.write_str("</div>")?;
        output.write_char('\n')
    }
}

/// ariel-rs does not handle line breaks inside node labels. It renders `\n`
/// (literal backslash-n from mermaid syntax) as verbatim text, and it
/// HTML-escapes `<br>` / `<br/>` so they appear as literal text too.
///
/// This function post-processes the SVG output: any `<tspan>` whose text
/// contains these patterns is split into multiple `<tspan>` elements with
/// `dy="1.2em"` offsets so the lines are stacked correctly.
fn fix_tspan_newlines(svg: &str) -> String {
    const OPEN: &str = "<tspan>";
    const CLOSE: &str = "</tspan>";
    // Patterns to treat as line-break separators inside tspan text.
    // ariel-rs HTML-escapes angle brackets, so <br> becomes &lt;br&gt;.
    const SPLIT_PATTERNS: &[&str] = &[
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
            // Normalise all break patterns to a single sentinel, then split.
            const SENTINEL: &str = "\x00";
            let normalized = SPLIT_PATTERNS
                .iter()
                .fold(text.to_owned(), |acc, pat| acc.replace(pat, SENTINEL));

            if normalized.contains(SENTINEL) {
                for (i, line) in normalized.split(SENTINEL).enumerate() {
                    if i == 0 {
                        result.push_str(OPEN);
                        result.push_str(line);
                        result.push_str(CLOSE);
                    } else {
                        result.push_str("<tspan x=\"0\" dy=\"1.2em\">");
                        result.push_str(line);
                        result.push_str(CLOSE);
                    }
                }
            } else {
                result.push_str(OPEN);
                result.push_str(text);
                result.push_str(CLOSE);
            }
            remaining = &remaining[close_pos + CLOSE.len()..];
        } else {
            // No closing tag — pass through unchanged.
            result.push_str(OPEN);
        }
    }
    result.push_str(remaining);
    result
}
