use std::collections::BTreeMap;

pub const SITE_HOME: &str = "/";
pub const SITE_NOT_FOUND: &str = "/404";
pub const SITE_ERROR: &str = "/error";
pub const DASH_HOME: &str = "/dash";
pub const DASH_LOGIN_SHOW: &str = "/dash/login";
pub const DASH_POSTS_LIST: &str = "/dash/posts";
pub const DASH_TASKS_LIST: &str = "/dash/tasks";
pub const DASH_TASKS_RUNS: &str = "/dash/tasks/{code}/runs";
pub const API_HEALTH: &str = "/api/health";
pub const API_SLUG: &str = "/api/slug";
pub const API_MARKDOWN: &str = "/api/markdown";
pub const API_MARKDOWN_TOC: &str = "/api/markdown/toc";
pub const API_TEMPLATE_PROBE: &str = "/api/template-probe";

const ROUTE_PAIRS: &[(&str, &str)] = &[
    ("site.home", SITE_HOME),
    ("site.not_found", SITE_NOT_FOUND),
    ("site.error", SITE_ERROR),
    ("dash.home", DASH_HOME),
    ("dash.login.show", DASH_LOGIN_SHOW),
    ("dash.posts.list", DASH_POSTS_LIST),
    ("dash.tasks.list", DASH_TASKS_LIST),
    ("api.health", API_HEALTH),
    ("api.slug", API_SLUG),
    ("api.markdown", API_MARKDOWN),
    ("api.markdown.toc", API_MARKDOWN_TOC),
    ("api.template_probe", API_TEMPLATE_PROBE),
];

pub fn route_table() -> BTreeMap<&'static str, &'static str> {
    ROUTE_PAIRS.iter().copied().collect()
}
