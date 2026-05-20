use std::{borrow::Cow, collections::BTreeMap, path::Path};

use anyhow::Result;
use minijinja::{context, value::Value, Environment, UndefinedBehavior};

const TEMPLATE_HEALTH: &str = "health.html";
const TEMPLATE_PROBE_PAGE: &str = "probe/page.html";
const TEMPLATE_PROBE_MACROS: &str = "probe/macros.html";
const TEMPLATE_PROBE_ITEM: &str = "probe/include/item.html";

fn template_sources() -> BTreeMap<&'static str, &'static str> {
    BTreeMap::from([
        (
            TEMPLATE_HEALTH,
            "<!doctype html><html><body><h1>{{ app_name }}</h1><p>{{ url_for('api.health') }}</p></body></html>",
        ),
        (
            TEMPLATE_PROBE_PAGE,
            "{% import \"macros.html\" as ui %}<section data-template=\"{{ url_for('site.home') }}\">{{ ui.row(app_name) }}</section>",
        ),
        (
            TEMPLATE_PROBE_MACROS,
            "{% macro row(label) %}{% include \"./include/item.html\" %}{% endmacro %}",
        ),
        (
            TEMPLATE_PROBE_ITEM,
            "<div>{{ label }}</div><div>{{ 1500|compact_number }}</div><div>{{ url_for('dash.home') }}</div>",
        ),
    ])
}

fn route_table() -> BTreeMap<&'static str, &'static str> {
    BTreeMap::from([
        ("api.health", "/api/health"),
        ("site.home", "/"),
        ("dash.home", "/dash"),
    ])
}

fn build_env() -> Environment<'static> {
    let templates = template_sources();
    let routes = route_table();

    let mut env = Environment::new();
    env.set_undefined_behavior(UndefinedBehavior::Lenient);
    env.set_loader(move |name| Ok(templates.get(name).map(|source| (*source).to_string())));
    env.set_path_join_callback(|name, parent| {
        Cow::Owned(resolve_template_import_path(name, parent))
    });
    env.add_function("url_for", move |name: String| -> Value {
        Value::from_safe_string(routes.get(name.trim()).copied().unwrap_or("#").to_string())
    });
    env.add_filter("compact_number", |value: i64| -> String {
        format_compact_number(value)
    });
    env
}

#[allow(dead_code)]
pub fn render_health(app_name: &str) -> Result<String> {
    let env = build_env();
    let tpl = env.get_template(TEMPLATE_HEALTH)?;
    Ok(tpl.render(context! { app_name => app_name })?)
}

pub fn render_template_probe(app_name: &str) -> Result<String> {
    let env = build_env();
    let tpl = env.get_template(TEMPLATE_PROBE_PAGE)?;
    Ok(tpl.render(context! { app_name => app_name })?)
}

fn resolve_template_import_path(name: &str, parent: &str) -> String {
    let name = name.trim();
    if name.is_empty() {
        return String::new();
    }

    if let Some(stripped) = name.strip_prefix('/') {
        return normalize_template_path(stripped);
    }

    if matches!(name, "." | "..") || name.starts_with("./") || name.starts_with("../") {
        let parent_dir = Path::new(parent.trim())
            .parent()
            .and_then(|path| path.to_str())
            .unwrap_or("");
        return normalize_template_path(&format!("{parent_dir}/{name}"));
    }

    let cleaned = normalize_template_path(name);
    if cleaned.is_empty() {
        return cleaned;
    }
    if cleaned.contains('/') {
        return cleaned;
    }

    let parent_dir = Path::new(parent.trim())
        .parent()
        .and_then(|path| path.to_str())
        .unwrap_or("");
    normalize_template_path(&format!("{parent_dir}/{cleaned}"))
}

fn normalize_template_path(input: &str) -> String {
    let mut parts = Vec::new();
    for part in input.split('/') {
        match part {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            value => parts.push(value),
        }
    }
    parts.join("/")
}

fn format_compact_number(value: i64) -> String {
    let abs = value.unsigned_abs() as f64;
    let sign = if value < 0 { "-" } else { "" };

    match abs {
        n if n >= 1_000_000_000.0 => format!(
            "{sign}{}",
            compact_number_with_unit(abs / 1_000_000_000.0, "b")
        ),
        n if n >= 1_000_000.0 => {
            format!("{sign}{}", compact_number_with_unit(abs / 1_000_000.0, "m"))
        }
        n if n >= 1_000.0 => format!("{sign}{}", compact_number_with_unit(abs / 1_000.0, "k")),
        _ => value.to_string(),
    }
}

fn compact_number_with_unit(value: f64, unit: &str) -> String {
    if value >= 10.0 {
        return format!("{:.0}{unit}", value.round());
    }
    let text = format!("{:.1}", (value * 10.0).round() / 10.0);
    format!("{}{unit}", text.trim_end_matches(".0"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn health_template_renders_named_route() {
        let html = render_health("swaves-rs").expect("render health");
        assert!(html.contains("<h1>swaves-rs</h1>"));
        assert!(html.contains("<p>/api/health</p>"));
    }

    #[test]
    fn probe_template_supports_import_relative_include_and_filter() {
        let html = render_template_probe("swaves-rs").expect("render template probe");
        assert_eq!(
            html,
            "<section data-template=\"/\"><div>swaves-rs</div><div>1.5k</div><div>/dash</div></section>"
        );
    }

    #[test]
    fn resolve_template_import_path_matches_go_style_rules() {
        assert_eq!(
            resolve_template_import_path("./include/item.html", "probe/page.html"),
            "probe/include/item.html"
        );
        assert_eq!(
            resolve_template_import_path("macros.html", "probe/page.html"),
            "probe/macros.html"
        );
        assert_eq!(
            resolve_template_import_path("/probe/include/item.html", "probe/page.html"),
            "probe/include/item.html"
        );
        assert_eq!(
            resolve_template_import_path("../shared/card.html", "probe/include/item.html"),
            "probe/shared/card.html"
        );
    }

    #[test]
    fn compact_number_matches_expected_examples() {
        assert_eq!(format_compact_number(999), "999");
        assert_eq!(format_compact_number(1_000), "1k");
        assert_eq!(format_compact_number(1_500), "1.5k");
        assert_eq!(format_compact_number(10_500), "11k");
        assert_eq!(format_compact_number(-1_500), "-1.5k");
    }

    #[test]
    fn loader_returns_template_not_found_for_unknown_templates() {
        let env = build_env();
        let err = env
            .get_template("missing.html")
            .expect_err("missing template should fail");
        assert_eq!(err.kind(), minijinja::ErrorKind::TemplateNotFound);
    }

    #[test]
    fn path_join_callback_applies_during_state_template_lookup() {
        let env = build_env();
        let template = env
            .get_template(TEMPLATE_PROBE_PAGE)
            .expect("load page template");
        let state = template.new_state();
        let nested = state
            .get_template("./include/item.html")
            .expect("resolve relative template from state");
        assert_eq!(nested.name(), TEMPLATE_PROBE_ITEM);
    }
}
