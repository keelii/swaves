use std::collections::BTreeMap;

use anyhow::Result;
use minijinja::{context, Environment};

pub fn render_health(app_name: &str) -> Result<String> {
    let mut env = Environment::new();
    env.add_template(
        "health.html",
        "<!doctype html><html><body><h1>{{ app_name }}</h1><p>{{ url_for('api.health') }}</p></body></html>",
    )?;

    env.add_function("url_for", |name: String| -> String {
        let mut routes = BTreeMap::new();
        routes.insert("api.health", "/api/health");
        routes.insert("site.home", "/");
        routes.insert("dash.home", "/dash");
        routes.get(name.as_str()).unwrap_or(&"#").to_string()
    });

    let tpl = env.get_template("health.html")?;
    Ok(tpl.render(context! { app_name => app_name })?)
}
