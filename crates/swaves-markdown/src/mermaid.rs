use std::fmt;

use comrak::adapters::CodefenceRendererAdapter;

use crate::util::escape_html;

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
        output.write_str("<div class=\"mermaid\" data-mermaid=\"true\">")?;
        output.write_str(&escape_html(code.trim_end()))?;
        output.write_str("</div>\n")
    }
}
