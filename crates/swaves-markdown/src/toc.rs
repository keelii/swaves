use crate::heading::Heading;
use crate::util::escape_html;

pub fn render(headings: &[Heading]) -> String {
    if headings.is_empty() {
        return String::new();
    }

    let mut html = String::from(
        "<div class=\"toc\"><span class=\"toc-toggle\" onclick=\"this.parentNode.classList.toggle('open')\">§</span>\n<h2 class=\"toc-title\">目录</h2>\n",
    );

    let base_level = headings[0].level;
    let mut current_level = base_level;
    html.push_str("<ol class=\"toc-list\">");

    for (index, heading) in headings.iter().enumerate() {
        if index > 0 {
            if heading.level > current_level {
                for _ in current_level..heading.level {
                    html.push_str("\n<ol>");
                }
            } else if heading.level < current_level {
                for _ in heading.level..current_level {
                    html.push_str("</li>\n</ol>");
                }
                html.push_str("</li>\n");
            } else {
                html.push_str("</li>\n");
            }
        }

        current_level = heading.level;
        html.push_str("<li><a href=\"#");
        html.push_str(&escape_html(&heading.id));
        html.push_str("\">");
        html.push_str(&escape_html(&heading.text));
        html.push_str("</a>");
    }

    html.push_str("</li>");
    while current_level > base_level {
        html.push_str("\n</ol></li>");
        current_level -= 1;
    }
    html.push_str("\n</ol>\n</div>\n");
    html
}
