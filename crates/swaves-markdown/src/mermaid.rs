use std::fmt;

use ariel_rs::theme::Theme;
use comrak::adapters::CodefenceRendererAdapter;

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
        let svg = ariel_rs::render(code.trim(), Theme::Default);
        output.write_str("<div class=\"mermaid\">")?;
        output.write_str(&svg)?;
        output.write_str("</div>")?;
        output.write_char('\n')
    }
}
