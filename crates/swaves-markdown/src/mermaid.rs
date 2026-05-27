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
        // Expand multi-line labels (\n / <br/>) before the CJK width pass so
        // that both post-processors see the final rect geometry.
        let svg = expand_multiline_nodes(&svg);
        let svg = fix_cjk_node_widths(&svg);
        output.write_str("<div class=\"mermaid\">")?;
        output.write_str(&svg)?;
        output.write_str("</div>")?;
        output.write_char('\n')
    }
}

// ── Multi-line label expansion ───────────────────────────────────────────────

/// ariel-rs uses a fixed single-line node height (`RECT_H ≈ 47.6 px`) and
/// passes label text through to the SVG `<tspan>` verbatim, treating `\n`
/// (backslash-n, two chars) and `&lt;br/&gt;` as regular characters.
///
/// This function post-processes the rendered SVG to properly support multi-line
/// node labels.  For every node whose innermost `<tspan>` text contains a line-
/// break pattern the following changes are applied:
///
/// 1. **Rect height** — `<rect class="basic label-container">` height is
///    increased by `(N-1) × FONT_SIZE × 1.1` px and `y` is shifted up by half
///    that amount so the box stays centred on the node's coordinate.
/// 2. **Label vertical centering** — the inner `<g class="label"
///    transform="translate(0,−…)">` y-offset is shifted up by the same half-
///    delta so the text block stays visually centred inside the enlarged box.
/// 3. **Tspan splitting** — the single `<tspan x="0" y="-0.1em" dy="1.1em">
///    <tspan>LINE1\nLINE2</tspan></tspan>` is replaced by one outer tspan per
///    line, each with `dy="1.1em"`, matching the mermaid.js `htmlLabels:false`
///    multi-line rendering.
/// 4. **TD edge endpoints** — for top-down flowcharts, edge-path start/end
///    y-coordinates that land on old node boundaries are shifted to the new
///    boundaries so arrows still connect at the correct box edge.
///
/// Break patterns recognised (all case variants):
/// - `\n`           — literal backslash-n emitted by ariel-rs
/// - `&lt;br/&gt;`  — ariel-rs HTML-escapes `<br/>` in labels
/// - `&lt;br&gt;`   — same for `<br>`
fn expand_multiline_nodes(svg: &str) -> String {
    /// ariel-rs FONT_SIZE constant (px).
    const FONT_SIZE: f64 = 16.0;
    /// Height added per extra line: FONT_SIZE × lineHeight = 16 × 1.1 = 17.6 px.
    const LINE_H: f64 = FONT_SIZE * 1.1;
    /// Edge-path trim: arrowhead tip ends 4 px before the node boundary.
    const EDGE_TRIM: f64 = 4.0;
    /// Tolerance for matching edge endpoint coordinates.
    const EPSILON: f64 = 1.5;
    /// Byte length of `d="` prefix.
    const D_ATTR_PREFIX_LEN: usize = 3;
    /// Byte length of `d="M` used to skip non-numeric marker paths.
    const D_ATTR_WITH_M_LEN: usize = 4;
    /// Tolerance for matching node centre coordinates.
    const COORD_TOL: f64 = 0.5;
    /// Window size (bytes) scanned ahead of a node's opening tag when extracting
    /// the bounding-box rect and label text.  A typical ariel-rs node block is
    /// well under 500 bytes; 2 000 bytes gives ample headroom for heavily-styled
    /// nodes while staying well below the next sibling's opening tag.
    const NODE_SCAN_WINDOW: usize = 2000;

    // ── Phase 1: collect geometry for every node with a multi-line label ──────
    // (cx, cy, old_half_h, n_lines) – nodes with n_lines ≥ 2 only.
    let mut node_info: Vec<(f64, f64, f64, usize)> = Vec::new();

    {
        let mut scan = svg;
        while let Some(rel) = scan.find("<g class=\"node") {
            scan = &scan[rel..];
            let tag_end = scan.find('>').unwrap_or(scan.len());
            let opening_tag = &scan[..tag_end + 1];

            if let (Some(cx), Some(cy)) = (
                extract_translate_x(opening_tag),
                extract_translate_y(opening_tag),
            ) {
                let win = (tag_end + 1 + NODE_SCAN_WINDOW).min(scan.len());
                let block = &scan[..win];
                if let Some(half_h) = extract_basic_rect_half_h(block) {
                    let text = collect_tspan_text(block);
                    let n = count_label_lines(&text);
                    if n >= 2 {
                        node_info.push((cx, cy, half_h, n));
                    }
                }
            }
            scan = &scan[2..];
        }
    }

    if node_info.is_empty() {
        return svg.to_string();
    }

    // ── Phase 2: rebuild SVG expanding multi-line nodes ───────────────────────
    let mut out = String::with_capacity(svg.len() + node_info.len() * 200);
    let mut remaining = svg;

    while let Some(pos) = remaining.find("<g class=\"node") {
        out.push_str(&remaining[..pos]);
        remaining = &remaining[pos..];

        let tag_end = remaining.find('>').unwrap_or(remaining.len());
        let opening_tag = &remaining[..tag_end + 1];
        let cx = extract_translate_x(opening_tag).unwrap_or(0.0);
        let cy = extract_translate_y(opening_tag).unwrap_or(0.0);

        let block_len = remaining[1..]
            .find("<g class=\"node")
            .map(|p| p + 1)
            .unwrap_or(remaining.len());
        let node_block = &remaining[..block_len];
        remaining = &remaining[block_len..];

        let maybe = node_info.iter().find(|&&(ncx, ncy, _, _)| {
            (ncx - cx).abs() < COORD_TOL && (ncy - cy).abs() < COORD_TOL
        });

        if let Some(&(_, _, _old_half_h, n_lines)) = maybe {
            let extra = (n_lines - 1) as f64 * LINE_H;
            out.push_str(&rewrite_multiline_node(node_block, extra));
        } else {
            out.push_str(node_block);
        }
    }
    out.push_str(remaining);

    // ── Phase 3: adjust TD edge-path y-endpoints ─────────────────────────────
    //
    // For TD flowcharts edges connect top/bottom node boundaries.  After
    // expanding a node vertically the old boundary y-coordinate no longer
    // matches the new box edge, so arrows would appear to end inside the rect.
    //
    // bottom_bounds: (old_bottom_y, +delta) — source-side, push outward (down)
    // top_bounds:    (old_top_minus_trim_y, -delta) — dest-side, pull inward (up)
    let bottom_bounds: Vec<(f64, f64)> = node_info
        .iter()
        .map(|&(_, cy, old_hh, n)| {
            let extra = (n - 1) as f64 * LINE_H;
            (cy + old_hh, extra / 2.0)
        })
        .collect();
    let top_bounds: Vec<(f64, f64)> = node_info
        .iter()
        .map(|&(_, cy, old_hh, n)| {
            let extra = (n - 1) as f64 * LINE_H;
            (cy - old_hh - EDGE_TRIM, -(extra / 2.0))
        })
        .collect();

    let svg = out;
    let mut out = String::with_capacity(svg.len());
    let mut remaining = svg.as_str();

    while let Some(pos) = remaining.find("d=\"M") {
        let after_m = pos + D_ATTR_WITH_M_LEN;
        let first_char = remaining[after_m..].chars().next().unwrap_or(' ');
        if !first_char.is_ascii_digit() && first_char != '-' {
            out.push_str(&remaining[..after_m]);
            remaining = &remaining[after_m..];
            continue;
        }
        out.push_str(&remaining[..pos + D_ATTR_PREFIX_LEN]);
        remaining = &remaining[pos + D_ATTR_PREFIX_LEN..];
        let quote_end = remaining.find('"').unwrap_or(remaining.len());
        let d_val = &remaining[..quote_end];
        out.push_str(&patch_td_edge_d(d_val, &bottom_bounds, &top_bounds, EPSILON));
        out.push('"');
        remaining = &remaining[quote_end + 1..];
    }
    out.push_str(remaining);
    out
}

/// Rewrite a single node block to support N lines:
/// - expand rect height and shift rect y upward
/// - shift the inner label-group translate upward to re-centre
/// - replace the single tspan with one tspan per line
fn rewrite_multiline_node(block: &str, extra_height: f64) -> String {
    let s = update_rect_yh(block, extra_height);
    let s = update_inner_label_y(&s, extra_height / 2.0);
    expand_tspan_multiline(&s)
}

/// Update `<rect class="basic label-container">` y and height for extra lines.
/// `new_y = old_y - extra/2`, `new_height = old_height + extra`.
fn update_rect_yh(block: &str, extra_height: f64) -> String {
    const MARKER: &str = "<rect class=\"basic label-container\"";
    let Some(rect_pos) = block.find(MARKER) else {
        return block.to_string();
    };
    let Some(close_rel) = block[rect_pos..].find('>') else {
        return block.to_string();
    };
    let rect_end = rect_pos + close_rel + 1;
    let rect_tag = &block[rect_pos..rect_end];

    let Some(y_val) = parse_f64_attr(rect_tag, "y") else {
        return block.to_string();
    };
    let Some(h_val) = parse_f64_attr(rect_tag, "height") else {
        return block.to_string();
    };

    let new_tag = replace_f64_attr(rect_tag, "y", y_val - extra_height / 2.0);
    let new_tag = replace_f64_attr(&new_tag, "height", h_val + extra_height);
    format!("{}{}{}", &block[..rect_pos], new_tag, &block[rect_end..])
}

/// Shift the inner label-group y-offset up by `shift_up` px.
///
/// ariel-rs emits two nested `<g class="label">` elements inside each node:
/// - outer: `transform="translate(0, 0)"` (note the space after the comma)
/// - inner: `transform="translate(0,-8.502)"` (negative y, no space)
///
/// We match only the inner one via the `translate(0,-` pattern (negative value).
fn update_inner_label_y(block: &str, shift_up: f64) -> String {
    const MARKER: &str = "transform=\"translate(0,-";

    let Some(pos) = block.find(MARKER) else {
        return block.to_string();
    };
    // `digits_start` points to the first digit of the magnitude (after the '-').
    let digits_start = pos + MARKER.len();
    let close_paren = block[digits_start..].find(')').unwrap_or(block[digits_start..].len());
    let digits_end = digits_start + close_paren;

    // Reconstruct the full negative number (include the '-' that is the last
    // character of MARKER).
    let num_start = digits_start - 1;
    let num_str = &block[num_start..digits_end]; // e.g. "-8.502"

    if let Ok(old_y) = num_str.parse::<f64>() {
        let new_y = old_y - shift_up;
        let prec = num_str
            .find('.')
            .map(|dot| num_str.len() - dot - 1)
            // ariel-rs emits the offset with 3 decimal places (e.g. "-8.502"),
            // so 3 is the right fallback when the value happens to be integral.
            .unwrap_or(3);
        format!(
            "{}{:.prec$}{}",
            &block[..num_start],
            new_y,
            &block[digits_end..]
        )
    } else {
        block.to_string()
    }
}

/// Replace `<tspan x="0" y="-0.1em" dy="1.1em"><tspan>LINE1\nLINE2</tspan></tspan>`
/// with one outer `<tspan dy="1.1em">` per line.
fn expand_tspan_multiline(block: &str) -> String {
    const OUTER_OPEN: &str = "<tspan x=\"0\" y=\"-0.1em\" dy=\"1.1em\"><tspan>";
    const CLOSE: &str = "</tspan></tspan>";

    let Some(outer_pos) = block.find(OUTER_OPEN) else {
        return block.to_string();
    };
    let content_start = outer_pos + OUTER_OPEN.len();
    let Some(close_rel) = block[content_start..].find(CLOSE) else {
        return block.to_string();
    };
    let content_end = content_start + close_rel;
    let text = &block[content_start..content_end];

    let lines = parse_label_lines(text);
    if lines.len() <= 1 {
        return block.to_string();
    }

    let mut new_tspan = String::new();
    for (i, line) in lines.iter().enumerate() {
        if i == 0 {
            new_tspan.push_str(&format!(
                "<tspan x=\"0\" y=\"-0.1em\" dy=\"1.1em\"><tspan>{line}</tspan></tspan>"
            ));
        } else {
            new_tspan.push_str(&format!(
                "<tspan x=\"0\" dy=\"1.1em\"><tspan>{line}</tspan></tspan>"
            ));
        }
    }

    format!(
        "{}{}{}",
        &block[..outer_pos],
        new_tspan,
        &block[content_end + CLOSE.len()..]
    )
}

/// Split a label text on all recognised break patterns, returning one string
/// per line.  The patterns are normalised before splitting so every variant
/// is treated uniformly.
fn parse_label_lines(text: &str) -> Vec<String> {
    // Use a NUL byte as an intermediate separator that cannot appear in SVG text.
    let norm = text
        .replace("\\n", "\x00") // literal backslash-n (two chars) from ariel-rs
        .replace("&lt;br/&gt;", "\x00")
        .replace("&lt;br&gt;", "\x00")
        .replace("&lt;BR/&gt;", "\x00")
        .replace("&lt;BR&gt;", "\x00");
    norm.split('\x00').map(|l| l.to_string()).collect()
}

/// Count the number of lines a label text splits into.
fn count_label_lines(text: &str) -> usize {
    parse_label_lines(text).len()
}

/// Adjust the y-coordinates of TD edge-path endpoints that land on old node
/// boundaries.  This is the vertical analogue of `patch_edge_d` (which handles
/// LR x-coordinates).
///
/// - `bottom_bounds`: `(old_bottom_y, +delta)` — source node's old bottom; push
///   the edge start downward.
/// - `top_bounds`: `(old_top_minus_trim_y, -delta)` — dest node's old
///   top-minus-trim; pull the edge end upward.
fn patch_td_edge_d(
    d: &str,
    bottom_bounds: &[(f64, f64)],
    top_bounds: &[(f64, f64)],
    eps: f64,
) -> String {
    if !d.starts_with('M') {
        return d.to_string();
    }
    let Some((x1, y1, first_end)) = parse_xy(&d[1..]) else {
        return d.to_string();
    };
    let Some(last_l) = d.rfind('L') else {
        return d.to_string();
    };
    let Some((xn, yn, _)) = parse_xy(&d[last_l + 1..]) else {
        return d.to_string();
    };

    let start_adj = bottom_bounds
        .iter()
        .find(|&&(by, _)| (by - y1).abs() < eps)
        .map(|&(_, adj)| adj)
        .unwrap_or(0.0);
    let end_adj = top_bounds
        .iter()
        .find(|&&(by, _)| (by - yn).abs() < eps)
        .map(|&(_, adj)| adj)
        .unwrap_or(0.0);

    if start_adj == 0.0 && end_adj == 0.0 {
        return d.to_string();
    }

    let middle = &d[1 + first_end..last_l];
    format!(
        "M{:.3},{:.3}{}L{:.3},{:.3}",
        x1,
        y1 + start_adj,
        middle,
        xn,
        yn + end_adj
    )
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
    /// Tolerance for matching a node's centre/half-width during the rebuild pass.
    const COORD_MATCH_TOLERANCE: f64 = 0.5;
    /// Byte length of the `d="` prefix in `d="M...`. Used when splitting the
    /// path attribute into the opening quote and the path-data string.
    const D_ATTR_PREFIX_LEN: usize = 3; // d="
    /// Byte length of `d="M` used when skipping non-numeric (marker) paths.
    const D_ATTR_WITH_M_LEN: usize = 4; // d="M

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
                // A typical ariel-rs node block (opening tag + rect + label group)
                // is well under 500 bytes; 2000 bytes provides ample headroom for
                // deeply-nested or heavily-styled nodes while staying far below the
                // next sibling's opening tag.
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
                            .find(|&&(ncx, nhw, _)| {
                                (ncx - cx).abs() < COORD_MATCH_TOLERANCE
                                    && (nhw - hw).abs() < COORD_MATCH_TOLERANCE
                            })
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

    // right_bounds: (old_right_boundary_x, positive_shift)  — arrows pushed outward
    // left_bounds:  (old_left_boundary_x,  negative_shift)  — arrows pulled inward
    let right_bounds: Vec<(f64, f64)> = node_info.iter().map(|&(cx, hw, d)| (cx + hw, d / 2.0)).collect();
    let left_bounds: Vec<(f64, f64)> = node_info.iter().map(|&(cx, hw, d)| (cx - hw - EDGE_TRIM, -(d / 2.0))).collect();

    let svg = out;
    let mut out = String::with_capacity(svg.len());
    let mut remaining = svg.as_str();

    while let Some(pos) = remaining.find("d=\"M") {
        // Only process numeric paths (edge paths start with `M{digit}` or `M-{digit}`),
        // not marker paths which use `M 0 0 ...` with spaces.
        let after_m = pos + D_ATTR_WITH_M_LEN;
        let first_data_char = remaining[after_m..].chars().next().unwrap_or(' ');
        if !first_data_char.is_ascii_digit() && first_data_char != '-' {
            out.push_str(&remaining[..pos + D_ATTR_WITH_M_LEN]);
            remaining = &remaining[pos + D_ATTR_WITH_M_LEN..];
            continue;
        }

        out.push_str(&remaining[..pos + D_ATTR_PREFIX_LEN]); // include 'd="'
        remaining = &remaining[pos + D_ATTR_PREFIX_LEN..]; // now starts with 'M...'

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

/// Extract the y component of `transform="translate(x, y)"` from a tag string.
fn extract_translate_y(tag: &str) -> Option<f64> {
    const PREFIX: &str = "transform=\"translate(";
    let start = tag.find(PREFIX)? + PREFIX.len();
    let comma = tag[start..].find(',')? + start;
    let y_start = comma + 1;
    let y_end = tag[y_start..].find(|c| c == ')' || c == '"')? + y_start;
    tag[y_start..y_end].trim().parse().ok()
}

/// Find `<rect class="basic label-container"` and return half-height (height/2).
fn extract_basic_rect_half_h(block: &str) -> Option<f64> {
    const MARKER: &str = "<rect class=\"basic label-container\"";
    let rect_pos = block.find(MARKER)?;
    let tag_end = block[rect_pos..].find('>')? + rect_pos;
    let rect_tag = &block[rect_pos..tag_end + 1];
    let h = parse_f64_attr(rect_tag, "height")?;
    Some(h / 2.0)
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
    // Count digits after the decimal point: total_length - dot_position - 1.
    // Falls back to 0 (integer) when there is no decimal point.
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

