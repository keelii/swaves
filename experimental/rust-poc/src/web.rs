use std::sync::{Arc, Mutex};

use axum::{
    extract::rejection::JsonRejection,
    extract::{Path, Query, State},
    http::StatusCode,
    response::{Html, IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use rusqlite::Connection;
use serde::{Deserialize, Serialize};
use tracing::{error, warn};

use crate::{cache::RuntimeCachePaths, db, htmlutil, markdown, routes, view};

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
        .route(routes::SITE_HOME, get(site_home))
        .route(routes::SITE_NOT_FOUND, get(site_not_found))
        .route(routes::SITE_ERROR, get(site_error))
        .route(routes::DASH_HOME, get(dash_home))
        .route(routes::DASH_LOGIN_SHOW, get(dash_login))
        .route(routes::DASH_POSTS_LIST, get(dash_posts))
        .route(routes::DASH_TASKS_LIST, get(dash_tasks))
        .route(routes::DASH_TASKS_RUNS, get(dash_task_runs))
        .route(routes::API_HEALTH, get(api_health))
        .route(routes::API_SLUG, get(api_slug))
        .route(routes::API_MARKDOWN, post(api_markdown))
        .route(routes::API_MARKDOWN_TOC, post(api_markdown_toc))
        .route(routes::API_TEMPLATE_PROBE, get(api_template_probe))
        .with_state(state)
}

async fn site_home(State(state): State<Arc<AppState>>) -> Html<String> {
    let routes = routes::route_table();
    let body = format!(
        "<h1>site</h1><p>sqlite: {}</p><p>cache: {}</p><ul><li><a href=\"{}\">dash</a></li><li><a href=\"{}\">api health</a></li><li><a href=\"{}\">template probe</a></li></ul>",
        htmlutil::escape(&state.sqlite_file),
        htmlutil::escape(&state.cache.root.display().to_string()),
        routes["dash.home"],
        routes["api.health"],
        routes["api.template_probe"],
    );
    Html(body)
}

async fn site_not_found() -> impl IntoResponse {
    (
        StatusCode::NOT_FOUND,
        Html(render_error_page(
            "not found",
            "the requested route is not available in the Rust POC response set.",
            "open / or /dash to continue exploring the current parity surface.",
        )),
    )
}

#[derive(Deserialize)]
struct ErrorQuery {
    message: Option<String>,
}

async fn site_error(Query(query): Query<ErrorQuery>) -> impl IntoResponse {
    let message = query
        .message
        .as_deref()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or("the Rust POC received an explicit site error probe.");
    (
        StatusCode::INTERNAL_SERVER_ERROR,
        Html(render_error_page(
            "site error",
            message,
            "check the worker logs and confirm the sqlite path is correct.",
        )),
    )
}

async fn dash_home() -> Html<String> {
    Html("<h1>dash</h1><p>experimental rust poc</p><ul><li><a href=\"/dash/login\">login</a></li><li><a href=\"/dash/posts\">posts</a></li><li><a href=\"/dash/tasks\">tasks</a></li></ul>".to_string())
}

async fn dash_login() -> Html<String> {
    Html("<h1>dash login</h1><p>Rust POC only exposes the read-side login screen.</p>".to_string())
}

async fn dash_posts(State(state): State<Arc<AppState>>) -> Result<Html<String>, PageError> {
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.posts.list",
            err,
            "failed to access sqlite connection for post list",
            "restart the worker and retry the request.",
        )
    })?;
    let posts = db::list_posts(&conn, 20).map_err(|err| {
        PageError::internal(
            "dash.posts.list",
            err,
            "failed to query posts from sqlite",
            "check that t_posts exists in the configured sqlite file.",
        )
    })?;

    let mut body = String::from("<h1>dash posts</h1><ul>");
    if posts.is_empty() {
        body.push_str("<li>no posts yet</li>");
    } else {
        for post in posts {
            body.push_str(&format!(
                "<li>#{} {} <small>{} / {} / {}</small></li>",
                post.id,
                htmlutil::escape(&post.title),
                htmlutil::escape(&post.slug),
                htmlutil::escape(&post.status),
                post.published_at
            ));
        }
    }
    body.push_str("</ul>");
    Ok(Html(body))
}

async fn dash_tasks(State(state): State<Arc<AppState>>) -> Result<Html<String>, PageError> {
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.list",
            err,
            "failed to access sqlite connection for task list",
            "restart the worker and retry the request.",
        )
    })?;
    let tasks = db::list_tasks(&conn, 20).map_err(|err| {
        PageError::internal(
            "dash.tasks.list",
            err,
            "failed to query tasks from sqlite",
            "check that t_tasks exists in the configured sqlite file.",
        )
    })?;

    let mut body = String::from("<h1>dash tasks</h1><ul>");
    if tasks.is_empty() {
        body.push_str("<li>no tasks yet</li>");
    } else {
        for task in tasks {
            body.push_str(&format!(
                "<li><a href=\"/dash/tasks/{}/runs\">{}</a> <small>enabled={} last_status={}</small></li>",
                htmlutil::escape(&task.code),
                htmlutil::escape(&task.name),
                task.enabled,
                htmlutil::escape(&task.last_status)
            ));
        }
    }
    body.push_str("</ul>");
    Ok(Html(body))
}

async fn dash_task_runs(
    Path(code): Path<String>,
    State(state): State<Arc<AppState>>,
) -> Result<Html<String>, PageError> {
    let code = code.trim().to_string();
    if code.is_empty() {
        return Err(PageError::not_found(
            "dash.tasks.runs",
            "task code is required for the task run list.",
            "open /dash/tasks and choose a concrete task code.",
        ));
    }

    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.runs",
            err,
            "failed to access sqlite connection for task runs",
            "restart the worker and retry the request.",
        )
    })?;
    let runs = db::list_task_runs(&conn, &code, 20).map_err(|err| {
        PageError::internal(
            "dash.tasks.runs",
            err,
            "failed to query task runs from sqlite",
            "check that t_task_runs exists in the configured sqlite file.",
        )
    })?;
    if runs.is_empty() {
        return Err(PageError::not_found(
            "dash.tasks.runs",
            format!("no task runs found for code `{}`.", code),
            "trigger the task first or inspect t_task_runs in sqlite.",
        ));
    }

    let mut body = format!(
        "<h1>task runs</h1><p>code: {}</p><ul>",
        htmlutil::escape(&code)
    );
    for run in runs {
        body.push_str(&format!(
            "<li>#{} {} <small>{} / {}</small></li>",
            run.id,
            htmlutil::escape(&run.status),
            htmlutil::escape(&run.message),
            run.created_at
        ));
    }
    body.push_str("</ul>");
    Ok(Html(body))
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

#[derive(Deserialize)]
struct SlugQuery {
    name: Option<String>,
}

#[derive(Serialize)]
struct DataResponse<T> {
    data: T,
}

async fn api_slug(Query(query): Query<SlugQuery>) -> Json<DataResponse<String>> {
    let slug = make_slug(query.name.as_deref().unwrap_or(""));
    Json(DataResponse { data: slug })
}

#[derive(Deserialize)]
struct MarkdownRequest {
    content: String,
    #[serde(default, rename = "toc")]
    _toc: Option<bool>,
}

async fn api_markdown(
    payload: Result<Json<MarkdownRequest>, JsonRejection>,
) -> Result<Json<DataResponse<String>>, ApiError> {
    let Json(body) = payload.map_err(|err| {
        ApiError::bad_request(
            "api.markdown",
            err,
            "invalid json body for markdown preview",
            r##"send JSON like {"content":"# title"}"##,
        )
    })?;
    Ok(Json(DataResponse {
        data: markdown::render_markdown(&body.content),
    }))
}

async fn api_markdown_toc(
    payload: Result<Json<MarkdownRequest>, JsonRejection>,
) -> Result<Json<DataResponse<String>>, ApiError> {
    let Json(body) = payload.map_err(|err| {
        ApiError::bad_request(
            "api.markdown.toc",
            err,
            "invalid json body for markdown toc preview",
            r##"send JSON like {"content":"# title"}"##,
        )
    })?;
    Ok(Json(DataResponse {
        data: markdown::render_markdown_toc(&body.content),
    }))
}

async fn api_template_probe() -> Result<Html<String>, PageError> {
    view::render_template_probe("swaves-rs")
        .map(Html)
        .map_err(|err| {
            PageError::internal(
                "api.template_probe",
                err,
                "failed to render MiniJinja template probe",
                "check template loader configuration and worker logs.",
            )
        })
}

#[derive(Serialize)]
struct ApiErrorBody {
    ok: bool,
    error: String,
    action: String,
}

struct ApiError {
    status: StatusCode,
    body: ApiErrorBody,
}

impl ApiError {
    fn bad_request(
        route: &'static str,
        err: impl std::fmt::Display,
        message: impl Into<String>,
        action: impl Into<String>,
    ) -> Self {
        warn!(route, error = %err, "request validation failed");
        Self {
            status: StatusCode::BAD_REQUEST,
            body: ApiErrorBody {
                ok: false,
                error: message.into(),
                action: action.into(),
            },
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (self.status, Json(self.body)).into_response()
    }
}

struct PageError {
    status: StatusCode,
    title: String,
    message: String,
    action: String,
}

impl PageError {
    fn internal(
        route: &'static str,
        err: impl std::fmt::Display,
        message: impl Into<String>,
        action: impl Into<String>,
    ) -> Self {
        error!(route, error = %err, "page handler failed");
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            title: "runtime error".to_string(),
            message: message.into(),
            action: action.into(),
        }
    }

    fn not_found(
        route: &'static str,
        message: impl Into<String>,
        action: impl Into<String>,
    ) -> Self {
        warn!(route, "page resource not found");
        Self {
            status: StatusCode::NOT_FOUND,
            title: "not found".to_string(),
            message: message.into(),
            action: action.into(),
        }
    }
}

impl IntoResponse for PageError {
    fn into_response(self) -> Response {
        (
            self.status,
            Html(render_error_page(&self.title, &self.message, &self.action)),
        )
            .into_response()
    }
}

fn make_slug(input: &str) -> String {
    let mut slug = String::with_capacity(input.len());
    let mut last_was_dash = false;
    for ch in input.trim().chars() {
        let normalized = ch.to_ascii_lowercase();
        if normalized.is_ascii_alphanumeric() {
            slug.push(normalized);
            last_was_dash = false;
            continue;
        }
        if !last_was_dash && !slug.is_empty() {
            slug.push('-');
            last_was_dash = true;
        }
    }
    slug.trim_matches('-').to_string()
}

fn render_error_page(title: &str, message: &str, action: &str) -> String {
    format!(
        "<h1>{}</h1><p>{}</p><p>action: {}</p>",
        htmlutil::escape(title),
        htmlutil::escape(message),
        htmlutil::escape(action)
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn make_slug_normalizes_basic_text() {
        assert_eq!(make_slug("Hello, Swaves!"), "hello-swaves");
        assert_eq!(make_slug("  a  b  "), "a-b");
        assert_eq!(make_slug("___"), "");
    }

    #[test]
    fn render_error_page_escapes_user_visible_text() {
        let html = render_error_page("bad <title>", "oops & retry", r#"click "home""#);
        assert!(html.contains("bad &lt;title&gt;"));
        assert!(html.contains("oops &amp; retry"));
        assert!(html.contains("click &quot;home&quot;"));
    }
}
