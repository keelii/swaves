use std::sync::Mutex;

use comrak::adapters::{HeadingAdapter, HeadingMeta};
use comrak::html::Anchorizer;
use comrak::nodes::{AstNode, NodeValue};

use crate::util::escape_html;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum HeadingIdStrategy {
    #[default]
    Unicode,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Heading {
    pub level: u8,
    pub text: String,
    pub id: String,
}

pub fn collect<'a>(root: &'a AstNode<'a>, _strategy: HeadingIdStrategy) -> Vec<Heading> {
    let mut anchorizer = Anchorizer::new();
    let mut headings = Vec::new();

    for node in root.descendants() {
        let NodeValue::Heading(heading) = &node.data.borrow().value else {
            continue;
        };

        let text = flatten_text(node);
        let source = if text.trim().is_empty() {
            "heading"
        } else {
            text.trim()
        };
        let id = anchorizer.anchorize(source);
        headings.push(Heading {
            level: heading.level,
            text,
            id,
        });
    }

    headings
}

pub struct PrecomputedHeadingAdapter {
    headings: Vec<Heading>,
    cursor: Mutex<usize>,
}

impl PrecomputedHeadingAdapter {
    pub fn new(headings: Vec<Heading>) -> Self {
        Self {
            headings,
            cursor: Mutex::new(0),
        }
    }
}

impl HeadingAdapter for PrecomputedHeadingAdapter {
    fn enter(
        &self,
        output: &mut dyn std::fmt::Write,
        heading: &HeadingMeta,
        _sourcepos: Option<comrak::nodes::Sourcepos>,
    ) -> std::fmt::Result {
        let mut cursor = self.cursor.lock().expect("heading cursor poisoned");
        let info = self
            .headings
            .get(*cursor)
            .filter(|item| item.level == heading.level);
        let id = info.map(|item| item.id.as_str()).unwrap_or_default();
        *cursor += 1;
        if id.is_empty() {
            write!(output, "<h{}>", heading.level)
        } else {
            write!(output, "<h{} id=\"{}\">", heading.level, escape_html(id),)
        }
    }

    fn exit(&self, output: &mut dyn std::fmt::Write, heading: &HeadingMeta) -> std::fmt::Result {
        write!(output, "</h{}>", heading.level)
    }
}

fn flatten_text<'a>(node: &'a AstNode<'a>) -> String {
    let mut text = String::new();
    for child in node.children() {
        flatten_node_text(child, &mut text);
    }
    text
}

fn flatten_node_text<'a>(node: &'a AstNode<'a>, text: &mut String) {
    match &node.data.borrow().value {
        NodeValue::Text(value) => text.push_str(value),
        NodeValue::Code(code) => text.push_str(&code.literal),
        NodeValue::Math(math) => text.push_str(&math.literal),
        NodeValue::LineBreak | NodeValue::SoftBreak => text.push(' '),
        _ => {
            for child in node.children() {
                flatten_node_text(child, text);
            }
        }
    }
}
