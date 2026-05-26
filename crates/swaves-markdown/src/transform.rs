use comrak::nodes::{AstNode, NodeValue};

use crate::util::escape_html;

pub fn rewrite_standalone_images<'a>(root: &'a AstNode<'a>) {
    let paragraphs: Vec<_> = root
        .descendants()
        .filter(|node| matches!(&node.data.borrow().value, NodeValue::Paragraph))
        .collect();

    for paragraph in paragraphs {
        let Some(html) = standalone_image_html(paragraph) else {
            continue;
        };

        while let Some(child) = paragraph.first_child() {
            child.detach();
        }
        paragraph.data.borrow_mut().value = NodeValue::Raw(html);
    }
}

fn standalone_image_html<'a>(paragraph: &'a AstNode<'a>) -> Option<String> {
    let image = only_child(paragraph)?;
    let NodeValue::Image(link) = &image.data.borrow().value else {
        return None;
    };

    let alt = image_alt_text(image);
    let src = escape_html(&link.url);
    let alt = escape_html(&alt);
    if link.title.is_empty() {
        return Some(format!("<p><img src=\"{src}\" alt=\"{alt}\"></p>\n"));
    }

    let title = escape_html(&link.title);
    Some(format!(
        "<figure class=\"fullwidth\"><img src=\"{src}\" alt=\"{alt}\"><figcaption>{title}</figcaption></figure>\n"
    ))
}

fn only_child<'a>(node: &'a AstNode<'a>) -> Option<&'a AstNode<'a>> {
    let first = node.first_child()?;
    if first.next_sibling().is_some() {
        return None;
    }
    Some(first)
}

fn image_alt_text<'a>(node: &'a AstNode<'a>) -> String {
    let mut text = String::new();
    for child in node.children() {
        collect_text(child, &mut text);
    }
    text
}

fn collect_text<'a>(node: &'a AstNode<'a>, text: &mut String) {
    match &node.data.borrow().value {
        NodeValue::Text(value) => text.push_str(value),
        NodeValue::Code(code) => text.push_str(&code.literal),
        NodeValue::SoftBreak | NodeValue::LineBreak => text.push(' '),
        _ => {
            for child in node.children() {
                collect_text(child, text);
            }
        }
    }
}
