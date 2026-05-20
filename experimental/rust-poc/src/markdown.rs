use pulldown_cmark::{html, Event, HeadingLevel, Options, Parser, Tag, TagEnd};

use crate::htmlutil;

pub fn render_markdown(input: &str) -> String {
    let parser = Parser::new_ext(input, markdown_options());
    let mut output = String::new();
    html::push_html(&mut output, parser);
    output
}

pub fn render_markdown_toc(input: &str) -> String {
    let headings = extract_headings(input);
    if headings.is_empty() {
        return String::new();
    }

    let mut output =
        String::from(r#"<div class="toc"><h2 class="toc-title">目录</h2><ol class="toc-list">"#);
    for heading in headings {
        output.push_str("<li>");
        output.push_str(&htmlutil::escape(&heading));
        output.push_str("</li>");
    }
    output.push_str("</ol></div>");
    output
}

fn extract_headings(input: &str) -> Vec<String> {
    let parser = Parser::new_ext(input, markdown_options());
    let mut headings = Vec::new();
    let mut current = String::new();
    let mut in_heading = false;

    for event in parser {
        match event {
            Event::Start(Tag::Heading { level, .. }) if heading_level_supported(level) => {
                in_heading = true;
                current.clear();
            }
            Event::End(TagEnd::Heading(level)) if heading_level_supported(level) => {
                if !current.trim().is_empty() {
                    headings.push(current.trim().to_string());
                }
                in_heading = false;
                current.clear();
            }
            Event::Text(text) | Event::Code(text) if in_heading => current.push_str(&text),
            _ => {}
        }
    }

    headings
}

fn heading_level_supported(level: HeadingLevel) -> bool {
    matches!(
        level,
        HeadingLevel::H1
            | HeadingLevel::H2
            | HeadingLevel::H3
            | HeadingLevel::H4
            | HeadingLevel::H5
            | HeadingLevel::H6
    )
}

fn markdown_options() -> Options {
    let mut options = Options::empty();
    options.insert(Options::ENABLE_TABLES);
    options.insert(Options::ENABLE_TASKLISTS);
    options.insert(Options::ENABLE_STRIKETHROUGH);
    options.insert(Options::ENABLE_HEADING_ATTRIBUTES);
    options
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn render_markdown_toc_returns_headings_only() {
        let toc = render_markdown_toc("# One\n\ntext\n\n## Two");
        assert!(toc.contains(r#"class="toc""#));
        assert!(toc.contains("<li>One</li>"));
        assert!(toc.contains("<li>Two</li>"));
        assert!(!toc.contains("text"));
    }

    #[test]
    fn render_markdown_toc_returns_empty_for_body_without_heading() {
        assert_eq!(render_markdown_toc("plain text"), "");
    }
}
