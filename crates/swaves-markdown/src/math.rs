use std::fmt;
use std::fmt::Write;

use comrak::html::{ChildRendering, Context, format_node_default};
use comrak::nodes::{Node, NodeValue};

use crate::options::RenderFailureMode;
use crate::util::escape_html;

#[derive(Debug, Clone)]
pub struct MathFormatterState {
    pub failure_mode: RenderFailureMode,
    pub error: Option<String>,
}

impl MathFormatterState {
    pub fn new(failure_mode: RenderFailureMode) -> Self {
        Self {
            failure_mode,
            error: None,
        }
    }
}

pub fn format_node(
    context: &mut Context<MathFormatterState>,
    node: Node<'_>,
    entering: bool,
) -> Result<ChildRendering, fmt::Error> {
    if let NodeValue::Math(math) = &node.data.borrow().value {
        if !entering {
            return Ok(ChildRendering::Skip);
        }

        let html = match render_math_html(&math.literal, math.display_math) {
            Ok(html) => html,
            Err(message) => {
                if matches!(context.user.failure_mode, RenderFailureMode::Error) {
                    context.user.error.get_or_insert(message.clone());
                }
                fallback_math_html(math.dollar_math, math.display_math, &math.literal)
            }
        };
        context.write_str(&html)?;
        return Ok(ChildRendering::Skip);
    }

    format_node_default(context, node, entering)
}

fn fallback_math_html(dollar_math: bool, display_math: bool, literal: &str) -> String {
    let source = if display_math {
        if dollar_math {
            format!("$${}$$", literal)
        } else {
            format!("```math\n{}\n```", literal)
        }
    } else if dollar_math {
        format!("${}$", literal)
    } else {
        format!("$`{}`$", literal)
    };
    escape_html(&source)
}

fn render_math_html(literal: &str, display_math: bool) -> Result<String, String> {
    #[cfg(feature = "math")]
    {
        use ratex_layout::{LayoutOptions, layout, to_display_list};
        use ratex_parser::parser::parse;
        use ratex_svg::{SvgOptions, render_to_svg};
        use ratex_types::math_style::MathStyle;

        let ast = parse(literal).map_err(|error| error.to_string())?;
        let style = if display_math {
            MathStyle::Display
        } else {
            MathStyle::Text
        };
        let options = LayoutOptions::default().with_style(style);
        let layout_box = layout(&ast, &options);
        let display_list = to_display_list(&layout_box);
        let svg = render_to_svg(
            &display_list,
            &SvgOptions {
                embed_glyphs: true,
                ..SvgOptions::default()
            },
        );
        let class_name = if display_math {
            "swaves-math swaves-math-display"
        } else {
            "swaves-math swaves-math-inline"
        };
        return Ok(format!(
            "<span class=\"{class_name}\" data-math=\"{}\">{svg}</span>",
            if display_math { "display" } else { "inline" }
        ));
    }

    #[cfg(not(feature = "math"))]
    {
        let _ = (literal, display_math);
        Err("math rendering feature is disabled".to_string())
    }
}
