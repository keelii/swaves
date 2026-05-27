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
        let svg = fix_cjk_node_widths(&svg);
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

// ── CJK spacing fix ──────────────────────────────────────────────────────────

/// ariel-rs uses `font_size * 0.5` as the width fallback for any character not
/// in its Latin/ASCII width table.  CJK ideographs fall into this bucket, but
/// their actual glyph width is `≈ font_size * 1.0` in every common CJK font.
/// The resulting under-measurement makes node boxes too narrow, so labels
/// overflow and sibling nodes appear cramped.
///
/// This function post-processes the rendered SVG to:
/// 1. Expand each `<rect class="basic label-container">` that wraps a CJK label
///    by `n_cjk_chars × font_size × 0.5` pixels (half the deficit per char).
/// 2. For left-right flowcharts, shift the first and last points of edge paths
///    whose endpoints land on a node boundary, so arrows still connect at the
///    new (wider) node edges.
fn fix_cjk_node_widths(svg: &str) -> String {
    /// ariel-rs hard-codes FONT_SIZE = 16 for flowchart nodes.
    const FONT_SIZE: f64 = 16.0;
    /// POINT_END_TRIM = 4 — edge path ends this many px before the node border.
    const EDGE_TRIM: f64 = 4.0;
    /// Coordinate-matching tolerance (px) for edge-endpoint detection.
    const EPSILON: f64 = 1.5;

    // ── Phase 1: collect geometry for every node that contains CJK text ──────

    // (cx, old_hw, delta) – only nodes with delta > 0 are stored.
    let mut node_info: Vec<(f64, f64, f64)> = Vec::new();

    {
        let mut scan = svg;
        while let Some(rel) = scan.find("<g class=\"node") {
            scan = &scan[rel..];
            let tag_end = scan.find('>').unwrap_or(scan.len().saturating_sub(1));
            let opening_tag = &scan[..tag_end + 1];

            if let Some(cx) = extract_translate_x(opening_tag) {
                // 2000-char window is generous enough for any single node block.
                let win = (tag_end + 1 + 2000).min(scan.len());
                let block = &scan[..win];
                if let Some((rx, _)) = extract_basic_rect_xw(block) {
                    let old_hw = -rx; // rect x attribute is always negative
                    let text = collect_tspan_text(block);
                    let cjk = text.chars().filter(|&c| is_cjk_char(c)).count();
                    if cjk > 0 {
                        node_info.push((cx, old_hw, (cjk as f64) * FONT_SIZE * 0.5));
                    }
                }
            }

            // Advance past the opening `<g` to avoid re-matching the same tag.
            scan = &scan[2..];
        }
    }

    if node_info.is_empty() {
        return svg.to_string();
    }

    // ── Phase 2: rebuild SVG expanding <rect> in each CJK node ───────────────

    let mut out = String::with_capacity(svg.len() + node_info.len() * 25);
    let mut remaining = svg;

    while let Some(pos) = remaining.find("<g class=\"node") {
        out.push_str(&remaining[..pos]);
        remaining = &remaining[pos..];

        // Determine delta for this node.
        let delta = {
            let tag_end = remaining.find('>').unwrap_or(remaining.len().saturating_sub(1));
            let opening_tag = &remaining[..tag_end + 1];
            extract_translate_x(opening_tag)
                .and_then(|cx| {
                    let win = (tag_end + 1 + 2000).min(remaining.len());
                    extract_basic_rect_xw(&remaining[..win]).and_then(|(rx, _)| {
                        let hw = -rx;
                        node_info
                            .iter()
                            .find(|&&(ncx, nhw, _)| (ncx - cx).abs() < 0.5 && (nhw - hw).abs() < 0.5)
                            .map(|&(_, _, d)| d)
                    })
                })
                .unwrap_or(0.0)
        };

        // This node's block runs until the next sibling node or end-of-SVG.
        let block_len = remaining[1..]
            .find("<g class=\"node")
            .map(|p| p + 1)
            .unwrap_or(remaining.len());
        let node_block = &remaining[..block_len];
        remaining = &remaining[block_len..];

        if delta > 0.0 {
            out.push_str(&expand_basic_rect(node_block, delta));
        } else {
            out.push_str(node_block);
        }
    }
    out.push_str(remaining);

    // ── Phase 3: adjust LR edge-path endpoints ───────────────────────────────
    //
    // For LR flowcharts the first path point sits on the source node's right
    // boundary and the last point on the dest node's left boundary minus TRIM.
    // We match those coordinates and apply the same half-delta shift so arrows
    // still land on the new (wider) box edges.
    //
    // For TD flowcharts edges are vertical; their x values equal node centres
    // (not boundaries), so they won't match any entry here and pass unchanged.

    let right_bounds: Vec<(f64, f64)> = node_info.iter().map(|&(cx, hw, d)| (cx + hw, d / 2.0)).collect();
    let left_bounds: Vec<(f64, f64)> = node_info.iter().map(|&(cx, hw, d)| (cx - hw - EDGE_TRIM, -(d / 2.0))).collect();

    let svg = out;
    let mut out = String::with_capacity(svg.len());
    let mut remaining = svg.as_str();

    while let Some(pos) = remaining.find("d=\"M") {
        // Only process numeric paths (edge paths start with `M{digit}` or `M-{digit}`),
        // not marker paths which use `M 0 0 ...` with spaces.
        let after_m = pos + 4;
        let first_data_char = remaining[after_m..].chars().next().unwrap_or(' ');
        if !first_data_char.is_ascii_digit() && first_data_char != '-' {
            out.push_str(&remaining[..pos + 4]);
            remaining = &remaining[pos + 4..];
            continue;
        }

        out.push_str(&remaining[..pos + 3]); // include 'd="'
        remaining = &remaining[pos + 3..]; // now starts with 'M...'

        let quote_end = remaining.find('"').unwrap_or(remaining.len());
        let d_val = &remaining[..quote_end];
        out.push_str(&patch_edge_d(d_val, &right_bounds, &left_bounds, EPSILON));
        out.push('"');
        remaining = &remaining[quote_end + 1..];
    }
    out.push_str(remaining);
    out
}

/// Returns `true` for characters in the major CJK Unicode blocks.
fn is_cjk_char(c: char) -> bool {
    matches!(
        c,
        '\u{3400}'..='\u{4DBF}'   // CJK Unified Ideographs Extension A
        | '\u{4E00}'..='\u{9FFF}' // CJK Unified Ideographs
        | '\u{F900}'..='\u{FAFF}' // CJK Compatibility Ideographs
        | '\u{3040}'..='\u{309F}' // Hiragana
        | '\u{30A0}'..='\u{30FF}' // Katakana
        | '\u{AC00}'..='\u{D7AF}' // Hangul Syllables
        | '\u{2E80}'..='\u{2EFF}' // CJK Radicals Supplement
        | '\u{3000}'..='\u{303F}' // CJK Symbols and Punctuation
    )
}

/// Concatenate all innermost `<tspan>TEXT</tspan>` text segments in a block.
fn collect_tspan_text(block: &str) -> String {
    let mut out = String::new();
    let mut rest = block;
    while let Some(pos) = rest.find("<tspan>") {
        rest = &rest[pos + 7..];
        if let Some(end) = rest.find("</tspan>") {
            out.push_str(&rest[..end]);
            rest = &rest[end + 8..];
        }
    }
    out
}

/// Extract the x component of `transform="translate(x, y)"` from a tag string.
fn extract_translate_x(tag: &str) -> Option<f64> {
    const PREFIX: &str = "transform=\"translate(";
    let start = tag.find(PREFIX)? + PREFIX.len();
    let end = tag[start..].find(|c| c == ',' || c == ')')? + start;
    tag[start..end].trim().parse().ok()
}

/// Find `<rect class="basic label-container"` and return `(x, width)`.
fn extract_basic_rect_xw(block: &str) -> Option<(f64, f64)> {
    const MARKER: &str = "<rect class=\"basic label-container\"";
    let rect_pos = block.find(MARKER)?;
    let tag_end = block[rect_pos..].find('>')? + rect_pos;
    let rect_tag = &block[rect_pos..tag_end + 1];
    let x = parse_f64_attr(rect_tag, "x")?;
    let w = parse_f64_attr(rect_tag, "width")?;
    Some((x, w))
}

/// Parse `attr="VALUE"` and return VALUE as f64.
fn parse_f64_attr(text: &str, attr: &str) -> Option<f64> {
    let key = format!("{attr}=\"");
    let start = text.find(key.as_str())? + key.len();
    let end = text[start..].find('"')? + start;
    text[start..end].parse().ok()
}

/// Return a copy of `tag` with the float value of `attr` replaced by `new_val`,
/// preserving the original decimal precision.
fn replace_f64_attr(tag: &str, attr: &str, new_val: f64) -> String {
    let key = format!("{attr}=\"");
    let Some(val_start) = tag.find(key.as_str()).map(|p| p + key.len()) else {
        return tag.to_string();
    };
    let Some(len) = tag[val_start..].find('"') else {
        return tag.to_string();
    };
    let val_end = val_start + len;
    let prec = tag[val_start..val_end]
        .find('.')
        .map(|dot| (val_end - val_start) - dot - 1)
        .unwrap_or(0);
    format!("{}{:.prec$}{}", &tag[..val_start], new_val, &tag[val_end..])
}

/// Return a copy of `block` with the first `<rect class="basic label-container">`
/// expanded symmetrically by `delta` pixels.
fn expand_basic_rect(block: &str, delta: f64) -> String {
    const MARKER: &str = "<rect class=\"basic label-container\"";
    let Some(rect_pos) = block.find(MARKER) else { return block.to_string() };
    let Some(close_rel) = block[rect_pos..].find('>') else { return block.to_string() };
    let rect_end = rect_pos + close_rel + 1;

    let rect_tag = &block[rect_pos..rect_end];
    let Some(x_val) = parse_f64_attr(rect_tag, "x") else { return block.to_string() };
    let Some(w_val) = parse_f64_attr(rect_tag, "width") else { return block.to_string() };

    // x is negative (left edge relative to centre); shift further left by delta/2.
    let new_tag = replace_f64_attr(rect_tag, "x", x_val - delta / 2.0);
    let new_tag = replace_f64_attr(&new_tag, "width", w_val + delta);

    format!("{}{}{}", &block[..rect_pos], new_tag, &block[rect_end..])
}

/// Adjust the start and/or end x-coordinate of a single SVG path `d` value
/// based on known right/left boundary adjustments.
fn patch_edge_d(
    d: &str,
    right_bounds: &[(f64, f64)],
    left_bounds: &[(f64, f64)],
    eps: f64,
) -> String {
    // Parse first coordinate (right after leading 'M').
    if !d.starts_with('M') {
        return d.to_string();
    }
    let Some((x1, y1, first_end)) = parse_xy(&d[1..]) else { return d.to_string() };

    // Find last 'L' and parse the coordinate following it.
    let Some(last_l) = d.rfind('L') else { return d.to_string() };
    let Some((xn, yn, _last_coord_len)) = parse_xy(&d[last_l + 1..]) else { return d.to_string() };

    let start_adj = right_bounds
        .iter()
        .find(|&&(bx, _)| (bx - x1).abs() < eps)
        .map(|&(_, adj)| adj)
        .unwrap_or(0.0);
    let end_adj = left_bounds
        .iter()
        .find(|&&(bx, _)| (bx - xn).abs() < eps)
        .map(|&(_, adj)| adj)
        .unwrap_or(0.0);

    if start_adj == 0.0 && end_adj == 0.0 {
        return d.to_string();
    }

    // Preserve the middle section; replace only the first and last coordinates.
    // 1 = len("M"), first_end is offset within d[1..], so total offset = 1+first_end.
    let middle = &d[1 + first_end..last_l];
    format!(
        "M{:.3},{:.3}{}L{:.3},{:.3}",
        x1 + start_adj,
        y1,
        middle,
        xn + end_adj,
        yn
    )
}

/// Parse `{x},{y}` at the start of `s` (after any leading whitespace has been
/// handled by the caller).  Returns `(x, y, consumed_bytes)`.
fn parse_xy(s: &str) -> Option<(f64, f64, usize)> {
    let comma = s.find(',')?;
    let x: f64 = s[..comma].parse().ok()?;
    let y_start = comma + 1;
    let y_end = s[y_start..]
        .find(|c: char| !c.is_ascii_digit() && c != '.' && c != '-')
        .map(|p| y_start + p)
        .unwrap_or(s.len());
    let y: f64 = s[y_start..y_end].parse().ok()?;
    Some((x, y, y_end))
}

