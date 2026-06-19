use crate::heading::HeadingIdStrategy;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum RenderFailureMode {
    #[default]
    Error,
    PreserveSource,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RenderOptions {
    pub generate_toc: bool,
    pub syntax_highlighting: bool,
    pub render_mermaid: bool,
    pub render_math: bool,
    pub heading_id_strategy: HeadingIdStrategy,
    pub render_failure_mode: RenderFailureMode,
}

impl Default for RenderOptions {
    fn default() -> Self {
        Self {
            generate_toc: true,
            syntax_highlighting: true,
            render_mermaid: true,
            render_math: true,
            heading_id_strategy: HeadingIdStrategy::Unicode,
            render_failure_mode: RenderFailureMode::Error,
        }
    }
}
