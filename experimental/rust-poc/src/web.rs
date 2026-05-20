use std::sync::{Arc, Mutex};

use axum::{extract::State, response::Html, routing::get, Json, Router};
use rusqlite::Connection;
use serde::Serialize;

use crate::{cache::RuntimeCachePaths, markdown, view};

#[derive(Clone)]
pub struct AppState {
    pub sqlite_file: String,
    pub cache: RuntimeCachePaths,
    pub db: Arc<Mutex<Connection>>,
}

impl AppState {
    pub fn new(sqlite_file: String, cache: RuntimeCachePaths, conn: Connection) -> Self {
        Self {
            sqlite_file,
            cache,
            db: Arc::new(Mutex::new(conn)),
        }
    }
}

pub fn router(state: Arc<AppState>) -> Router {
    Router::new()
        .route("/", get(site_home))
        .route("/dash", get(dash_home))
        .route("/api/health", get(api_health))
        .route("/api/markdown", get(api_markdown_preview))
        .with_state(state)
}

async fn site_home(State(state): State<Arc<AppState>>) -> Html<String> {
    let body = format!(
        "<h1>site</h1><p>sqlite: {}</p><p>cache: {}</p>",
        html_escape(&state.sqlite_file),
        html_escape(&state.cache.root.display().to_string())
    );
    Html(body)
}

async fn dash_home() -> Html<String> {
    Html("<h1>dash</h1><p>experimental rust poc</p>".to_string())
}

#[derive(Serialize)]
struct Health {
    ok: bool,
    runtime: &'static str,
}

async fn api_health() -> Json<Health> {
    Json(Health {
        ok: true,
        runtime: "rust-poc",
    })
}

async fn api_markdown_preview() -> Html<String> {
    let markdown = "### swaves-rs\n\n- site\n- dash\n- api";
    Html(markdown::render_markdown(markdown))
}

#[allow(dead_code)]
async fn template_probe() -> Html<String> {
    match view::render_health("swaves-rs") {
        Ok(html) => Html(html),
        Err(err) => Html(format!("template error: {}", html_escape(&err.to_string()))),
    }
}

fn html_escape(input: &str) -> String {
    let mut escaped = String::with_capacity(input.len());
    for ch in input.chars() {
        match ch {
            '&' => escaped.push_str("&amp;"),
            '<' => escaped.push_str("&lt;"),
            '>' => escaped.push_str("&gt;"),
            '"' => escaped.push_str("&quot;"),
            '\'' => escaped.push_str("&#39;"),
            _ => escaped.push(ch),
        }
    }
    escaped
}
