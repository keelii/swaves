use std::fmt::{self, Write};
use std::sync::{Arc, Mutex};

use comrak::adapters::CodefenceRendererAdapter;

use crate::options::RenderFailureMode;
use crate::util::escape_html;

pub struct MermaidRenderer {
    failure_mode: RenderFailureMode,
    error: Arc<Mutex<Option<String>>>,
}

impl MermaidRenderer {
    pub fn new(failure_mode: RenderFailureMode, error: Arc<Mutex<Option<String>>>) -> Self {
        Self {
            failure_mode,
            error,
        }
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
        #[cfg(feature = "mermaid")]
        match mermaid_rs_renderer::render(code) {
            Ok(svg) => {
                output.write_str("<figure class=\"swaves-mermaid\" data-mermaid=\"true\">")?;
                output.write_str(&svg)?;
                output.write_str("</figure>\n")
            }
            Err(error) => {
                self.error
                    .lock()
                    .expect("mermaid error lock poisoned")
                    .get_or_insert_with(|| error.to_string());
                render_fallback(output, self.failure_mode, code)
            }
        }

        #[cfg(not(feature = "mermaid"))]
        {
            render_fallback(output, self.failure_mode, code)
        }
    }
}

fn render_fallback(
    output: &mut dyn Write,
    failure_mode: RenderFailureMode,
    code: &str,
) -> fmt::Result {
    match failure_mode {
        RenderFailureMode::Error | RenderFailureMode::PreserveSource => {
            write!(
                output,
                "<pre class=\"mermaid\">{}</pre>\n",
                escape_html(code)
            )
        }
    }
}
