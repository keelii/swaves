use std::collections::BTreeMap;

use comrak::adapters::HeadingAdapter;
use comrak::html::format_document_with_formatter;
use comrak::options::Plugins;
use comrak::{Arena, Options, parse_document};
use serde_json::Value;

use crate::error::{Error, Result};
use crate::frontmatter::split;
use crate::heading::{PrecomputedHeadingAdapter, collect};
use crate::highlight;
use crate::math::MathFormatterState;
use crate::mermaid::MermaidRenderer;
use crate::options::{RenderFailureMode, RenderOptions};
use crate::toc;
use crate::transform;

#[derive(Debug, Clone, PartialEq)]
pub struct RenderResult {
    pub metadata: BTreeMap<String, Value>,
    pub markdown: String,
    pub html: String,
    pub toc_html: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TocResult {
    pub toc_html: String,
}

pub fn render(markdown: &str, options: &RenderOptions) -> Result<RenderResult> {
    let parsed = split(markdown)?;
    let rendered = render_document(&parsed.markdown, options)?;
    Ok(RenderResult {
        metadata: parsed.metadata,
        markdown: parsed.markdown,
        html: rendered.html,
        toc_html: rendered.toc_html,
    })
}

pub fn render_toc(markdown: &str) -> Result<TocResult> {
    let options = RenderOptions::default();
    let parsed = split(markdown)?;
    let rendered = render_document(&parsed.markdown, &options)?;
    Ok(TocResult {
        toc_html: rendered.toc_html,
    })
}

struct DocumentRender {
    html: String,
    toc_html: String,
}

fn render_document(markdown: &str, options: &RenderOptions) -> Result<DocumentRender> {
    let arena = Arena::new();
    let comrak_options = build_options();
    let root = parse_document(&arena, markdown, &comrak_options);
    transform::rewrite_standalone_images(root);

    let headings = collect(root, options.heading_id_strategy);
    let toc_html = if options.generate_toc {
        toc::render(&headings)
    } else {
        String::new()
    };

    let heading_adapter = PrecomputedHeadingAdapter::new(headings);
    let mermaid_renderer = MermaidRenderer::new();
    let highlighter = highlight::adapter();

    let mut plugins = Plugins::default();
    configure_plugins(
        &mut plugins,
        options,
        &heading_adapter,
        &mermaid_renderer,
        &highlighter,
    );

    let mut html = String::new();
    let math_state = format_document_with_formatter(
        root,
        &comrak_options,
        &mut html,
        &plugins,
        crate::math::format_node,
        MathFormatterState::new(options.render_failure_mode),
    )?;

    if let Some(message) = math_state.error {
        if matches!(options.render_failure_mode, RenderFailureMode::Error) {
            return Err(Error::Math(message));
        }
    }

    Ok(DocumentRender { html, toc_html })
}

fn build_options() -> Options<'static> {
    let mut options = Options::default();
    options.extension.strikethrough = true;
    options.extension.table = true;
    options.extension.autolink = true;
    options.extension.tasklist = true;
    options.extension.footnotes = true;
    options.extension.math_dollars = true;
    options.extension.math_code = true;
    options.parse.smart = true;
    options.render.r#unsafe = true;
    options.render.github_pre_lang = true;
    options
}

fn configure_plugins<'a>(
    plugins: &mut Plugins<'a>,
    options: &RenderOptions,
    heading_adapter: &'a dyn HeadingAdapter,
    mermaid_renderer: &'a MermaidRenderer,
    highlighter: &'a dyn comrak::adapters::SyntaxHighlighterAdapter,
) {
    plugins.render.heading_adapter = Some(heading_adapter);
    if options.render_mermaid {
        plugins
            .render
            .codefence_renderers
            .insert("mermaid".to_string(), mermaid_renderer);
    }
    if options.syntax_highlighting {
        plugins.render.codefence_syntax_highlighter = Some(highlighter);
    }
}
