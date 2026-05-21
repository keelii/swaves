use std::sync::{Arc, Mutex};

use axum::{
    extract::rejection::JsonRejection,
    extract::{Path, Query, State},
    http::StatusCode,
    response::{Html, IntoResponse, Redirect, Response},
    routing::{get, post},
    Form, Json, Router,
};
use rusqlite::Connection;
use serde::{Deserialize, Serialize};
use tracing::{error, warn};

use crate::{cache::RuntimeCachePaths, db, htmlutil, jobs, markdown, routes, view};

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
        .route(
            routes::DASH_TASKS_NEW,
            get(dash_task_new).post(dash_create_task),
        )
        .route(
            routes::DASH_TASKS_EDIT,
            get(dash_task_edit).post(dash_update_task),
        )
        .route(routes::DASH_TASKS_DELETE, post(dash_delete_task))
        .route(routes::DASH_TASKS_TRIGGER, post(dash_trigger_task))
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
    Html(format!(
        "<h1>dash</h1><p>experimental rust poc</p><ul><li><a href=\"{}\">login</a></li><li><a href=\"{}\">posts</a></li><li><a href=\"{}\">tasks</a></li></ul>",
        routes::DASH_LOGIN_SHOW,
        routes::DASH_POSTS_LIST,
        routes::DASH_TASKS_LIST
    ))
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

    let mut body = format!(
        "<h1>dash tasks</h1><p><a href=\"{}\">new task</a></p><ul>",
        routes::DASH_TASKS_NEW
    );
    if tasks.is_empty() {
        body.push_str("<li>no tasks yet</li>");
    } else {
        for task in tasks {
            let runs_path = routes::dash_task_runs_path(&task.code);
            let trigger_path = routes::dash_task_trigger_path(&task.code);
            let edit_path = routes::dash_task_edit_path(task.id);
            let delete_action = if task.kind == TASK_KIND_USER {
                format!(
                    "<form method=\"post\" action=\"{}\" style=\"display:inline\"><button type=\"submit\">delete</button></form>",
                    htmlutil::escape(&routes::dash_task_delete_path(task.id))
                )
            } else {
                "<small>internal task</small>".to_string()
            };
            body.push_str(&format!(
                "<li><a href=\"{}\">{}</a> <small>schedule={} enabled={} kind={} last_status={}</small> \
                 <a href=\"{}\">edit</a> {} \
                 <form method=\"post\" action=\"{}\" style=\"display:inline\"><button type=\"submit\">trigger</button></form></li>",
                htmlutil::escape(&runs_path),
                htmlutil::escape(&task.name),
                htmlutil::escape(task_schedule_display(&task.schedule)),
                task.enabled,
                htmlutil::escape(task_kind_label(task.kind)),
                htmlutil::escape(&task.last_status),
                htmlutil::escape(&edit_path),
                delete_action,
                htmlutil::escape(&trigger_path)
            ));
        }
    }
    body.push_str("</ul>");
    Ok(Html(body))
}

async fn dash_task_new() -> Html<String> {
    Html(render_task_form(
        "new task",
        routes::DASH_TASKS_NEW,
        "create task",
        &TaskFormState::default(),
        true,
    ))
}

async fn dash_create_task(
    State(state): State<Arc<AppState>>,
    Form(form): Form<TaskFormInput>,
) -> Result<Redirect, PageError> {
    let task = build_task_create_mutation(form).map_err(|(message, action)| {
        PageError::bad_request("dash.tasks.create", message, action)
    })?;
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.create",
            err,
            "failed to access sqlite connection for task create",
            "restart the worker and retry the request.",
        )
    })?;
    db::create_task(&conn, &task).map_err(|err| {
        PageError::internal(
            "dash.tasks.create",
            err,
            "failed to create task in sqlite",
            "check whether the task code already exists and the sqlite file is writable.",
        )
    })?;
    Ok(Redirect::to(routes::DASH_TASKS_LIST))
}

async fn dash_task_edit(
    Path(task_id): Path<i64>,
    State(state): State<Arc<AppState>>,
) -> Result<Html<String>, PageError> {
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.edit",
            err,
            "failed to access sqlite connection for task edit",
            "restart the worker and retry the request.",
        )
    })?;
    let Some(task) = db::get_task_by_id(&conn, task_id).map_err(|err| {
        PageError::internal(
            "dash.tasks.edit",
            err,
            "failed to query task from sqlite",
            "check that t_tasks exists in the configured sqlite file.",
        )
    })?
    else {
        return Err(PageError::not_found(
            "dash.tasks.edit",
            format!("task id `{}` does not exist.", task_id),
            "open /dash/tasks and choose an existing task before editing.",
        ));
    };

    Ok(Html(render_task_form(
        "edit task",
        &routes::dash_task_edit_path(task.id),
        "update task",
        &TaskFormState::from_task(&task),
        false,
    )))
}

async fn dash_update_task(
    Path(task_id): Path<i64>,
    State(state): State<Arc<AppState>>,
    Form(form): Form<TaskFormInput>,
) -> Result<Redirect, PageError> {
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.update",
            err,
            "failed to access sqlite connection for task update",
            "restart the worker and retry the request.",
        )
    })?;
    let Some(existing) = db::get_task_by_id(&conn, task_id).map_err(|err| {
        PageError::internal(
            "dash.tasks.update",
            err,
            "failed to query task from sqlite",
            "check that t_tasks exists in the configured sqlite file.",
        )
    })?
    else {
        return Err(PageError::not_found(
            "dash.tasks.update",
            format!("task id `{}` does not exist.", task_id),
            "open /dash/tasks and choose an existing task before updating.",
        ));
    };

    let task = build_task_update_mutation(&existing, form);
    db::update_task(&conn, task_id, &task).map_err(|err| {
        PageError::internal(
            "dash.tasks.update",
            err,
            format!("failed to update task `{}`.", existing.code),
            "check whether the sqlite file is writable and retry the request.",
        )
    })?;
    Ok(Redirect::to(routes::DASH_TASKS_LIST))
}

async fn dash_delete_task(
    Path(task_id): Path<i64>,
    State(state): State<Arc<AppState>>,
) -> Result<Redirect, PageError> {
    let conn = state.db.lock().map_err(|err| {
        PageError::internal(
            "dash.tasks.delete",
            err,
            "failed to access sqlite connection for task delete",
            "restart the worker and retry the request.",
        )
    })?;
    let Some(task) = db::get_task_by_id(&conn, task_id).map_err(|err| {
        PageError::internal(
            "dash.tasks.delete",
            err,
            "failed to query task from sqlite",
            "check that t_tasks exists in the configured sqlite file.",
        )
    })?
    else {
        return Err(PageError::not_found(
            "dash.tasks.delete",
            format!("task id `{}` does not exist.", task_id),
            "open /dash/tasks and choose an existing task before deleting.",
        ));
    };
    if task.kind == TASK_KIND_INTERNAL {
        return Err(PageError::bad_request(
            "dash.tasks.delete",
            "internal task cannot be deleted.",
            "create a user task if you need a removable task entry.",
        ));
    }
    db::soft_delete_task(&conn, task_id).map_err(|err| {
        PageError::internal(
            "dash.tasks.delete",
            err,
            format!("failed to delete task `{}`.", task.code),
            "check whether the sqlite file is writable and retry the request.",
        )
    })?;
    Ok(Redirect::to(routes::DASH_TASKS_LIST))
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
    body.push_str(&format!(
        "</ul><form method=\"post\" action=\"/dash/tasks/{}/trigger\"><button type=\"submit\">trigger again</button></form><p><a href=\"{}\">back to task list</a></p>",
        htmlutil::escape(&routes::dash_task_trigger_path(&code)),
        routes::DASH_TASKS_LIST
    ));
    Ok(Html(body))
}

async fn dash_trigger_task(
    Path(code): Path<String>,
    State(state): State<Arc<AppState>>,
) -> Result<Redirect, PageError> {
    let code = code.trim().to_string();
    if code.is_empty() {
        return Err(PageError::not_found(
            "dash.tasks.trigger",
            "task code is required for manual trigger.",
            "open /dash/tasks and trigger a concrete task.",
        ));
    }

    let task = {
        let conn = state.db.lock().map_err(|err| {
            PageError::internal(
                "dash.tasks.trigger",
                err,
                "failed to access sqlite connection for manual task trigger",
                "restart the worker and retry the request.",
            )
        })?;
        db::get_task_by_code(&conn, &code).map_err(|err| {
            PageError::internal(
                "dash.tasks.trigger",
                err,
                "failed to query task from sqlite",
                "check that t_tasks exists in the configured sqlite file.",
            )
        })?
    };

    let Some(task) = task else {
        return Err(PageError::not_found(
            "dash.tasks.trigger",
            format!("task code `{}` does not exist.", code),
            "open /dash/tasks and choose an existing task before triggering.",
        ));
    };

    jobs::trigger_task(state, task.code.as_str())
        .await
        .map_err(|err| {
            PageError::internal(
                "dash.tasks.trigger",
                err,
                format!("failed to trigger task `{}`.", task.code),
                "check whether the task is registered in the Rust POC job registry.",
            )
        })?;

    Ok(Redirect::to(routes::DASH_TASKS_LIST))
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

#[derive(Debug)]
struct PageError {
    status: StatusCode,
    title: String,
    message: String,
    action: String,
}

impl PageError {
    fn bad_request(
        route: &'static str,
        message: impl Into<String>,
        action: impl Into<String>,
    ) -> Self {
        warn!(route, "page request validation failed");
        Self {
            status: StatusCode::BAD_REQUEST,
            title: "bad request".to_string(),
            message: message.into(),
            action: action.into(),
        }
    }

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

const TASK_KIND_INTERNAL: i64 = 0;
const TASK_KIND_USER: i64 = 1;
const TASK_SCHEDULE_OPTIONS: [(&str, &str); 5] = [
    ("@hourly", "每小时"),
    ("@daily", "每天"),
    ("@weekly", "每周"),
    ("@monthly", "每月"),
    ("@yearly", "每年"),
];

#[derive(Default)]
struct TaskFormState {
    code: String,
    name: String,
    description: String,
    schedule: String,
    enabled: bool,
    kind: i64,
}

impl TaskFormState {
    fn from_task(task: &db::TaskDetail) -> Self {
        Self {
            code: task.code.clone(),
            name: task.name.clone(),
            description: task.description.clone(),
            schedule: task.schedule.clone(),
            enabled: task.enabled == 1,
            kind: task.kind,
        }
    }
}

#[derive(Debug, Deserialize)]
struct TaskFormInput {
    #[serde(default)]
    code: String,
    #[serde(default)]
    name: String,
    #[serde(default)]
    description: String,
    #[serde(default)]
    schedule: String,
    enabled: Option<String>,
    kind: Option<String>,
}

fn build_task_create_mutation(
    form: TaskFormInput,
) -> Result<db::TaskMutation, (&'static str, &'static str)> {
    let code = form.code.trim().to_string();
    let name = form.name.trim().to_string();
    let schedule = form.schedule.trim().to_string();
    if code.is_empty() {
        return Err((
            "task code is required.",
            "fill in a stable task code before creating the task.",
        ));
    }
    if name.is_empty() {
        return Err((
            "task name is required.",
            "fill in a visible task name before creating the task.",
        ));
    }
    if schedule.is_empty() {
        return Err((
            "task schedule is required.",
            "choose a preset schedule or enter a cron expression before creating the task.",
        ));
    }
    Ok(db::TaskMutation {
        code,
        name,
        description: form.description.trim().to_string(),
        schedule,
        enabled: bool_to_i64(form.enabled.is_some()),
        kind: task_kind_from_form(form.kind.as_deref()),
    })
}

fn build_task_update_mutation(existing: &db::TaskDetail, form: TaskFormInput) -> db::TaskMutation {
    db::TaskMutation {
        code: existing.code.clone(),
        name: form.name.trim().to_string(),
        description: form.description.trim().to_string(),
        schedule: form.schedule.trim().to_string(),
        enabled: bool_to_i64(form.enabled.is_some()),
        kind: task_kind_from_form(form.kind.as_deref()),
    }
}

fn task_kind_from_form(value: Option<&str>) -> i64 {
    match value.unwrap_or("0").trim() {
        "1" => TASK_KIND_USER,
        _ => TASK_KIND_INTERNAL,
    }
}

fn bool_to_i64(value: bool) -> i64 {
    if value {
        1
    } else {
        0
    }
}

fn task_schedule_display(schedule: &str) -> &str {
    for (value, label) in TASK_SCHEDULE_OPTIONS {
        if value == schedule.trim() {
            return label;
        }
    }
    schedule
}

fn task_kind_label(kind: i64) -> &'static str {
    match kind {
        TASK_KIND_USER => "user",
        _ => "internal",
    }
}

fn render_task_form(
    title: &str,
    action: &str,
    submit_label: &str,
    task: &TaskFormState,
    code_editable: bool,
) -> String {
    let mut schedule_options = String::new();
    for (value, label) in TASK_SCHEDULE_OPTIONS {
        let selected = if task.schedule == value {
            " selected"
        } else {
            ""
        };
        schedule_options.push_str(&format!(
            "<option value=\"{}\"{}>{}</option>",
            htmlutil::escape(value),
            selected,
            htmlutil::escape(label)
        ));
    }
    let internal_selected = if task.kind == TASK_KIND_INTERNAL {
        " selected"
    } else {
        ""
    };
    let user_selected = if task.kind == TASK_KIND_USER {
        " selected"
    } else {
        ""
    };
    let checked = if task.enabled { " checked" } else { "" };
    let code_input = if code_editable {
        format!(
            "<input name=\"code\" value=\"{}\" />",
            htmlutil::escape(&task.code)
        )
    } else {
        format!(
            "<input name=\"code\" value=\"{}\" readonly /><small> code is immutable in Go too.</small>",
            htmlutil::escape(&task.code)
        )
    };

    format!(
        "<h1>{}</h1><form method=\"post\" action=\"{}\">\
         <p><label>code {}</label></p>\
         <p><label>name <input name=\"name\" value=\"{}\" /></label></p>\
         <p><label>description <textarea name=\"description\">{}</textarea></label></p>\
         <p><label>schedule <input name=\"schedule\" value=\"{}\" list=\"task-schedules\" /></label></p>\
         <datalist id=\"task-schedules\">{}</datalist>\
         <p><label>enabled <input type=\"checkbox\" name=\"enabled\" value=\"1\"{}/></label></p>\
         <p><label>kind <select name=\"kind\"><option value=\"0\"{}>internal</option><option value=\"1\"{}>user</option></select></label></p>\
         <p><button type=\"submit\">{}</button> <a href=\"{}\">cancel</a></p></form>",
        htmlutil::escape(title),
        htmlutil::escape(action),
        code_input,
        htmlutil::escape(&task.name),
        htmlutil::escape(&task.description),
        htmlutil::escape(&task.schedule),
        schedule_options,
        checked,
        internal_selected,
        user_selected,
        htmlutil::escape(submit_label),
        routes::DASH_TASKS_LIST
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{cache::RuntimeCachePaths, db::TaskDefinition};
    use rusqlite::Connection;

    fn build_state() -> Arc<AppState> {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(include_str!(concat!(env!("OUT_DIR"), "/initial_sql.sql")))
            .expect("initialize schema");
        Arc::new(AppState::new(
            "/tmp/swaves-poc-web-test.sqlite".to_string(),
            RuntimeCachePaths {
                root: "/tmp/swaves-poc-web-cache".into(),
                updater_root: "/tmp/swaves-poc-web-cache/updater".into(),
            },
            conn,
        ))
    }

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

    #[tokio::test]
    async fn dash_trigger_task_redirects_after_manual_run() {
        let state = build_state();
        {
            let conn = state.db.lock().expect("lock sqlite");
            db::ensure_builtin_task(
                &conn,
                &TaskDefinition {
                    code: "clear_notifications",
                    name: "清理过期通知",
                    description: "按保留天数清理过期通知",
                    schedule: "@daily",
                    enabled: 1,
                    kind: 0,
                },
            )
            .expect("seed task");
        }

        let redirect = dash_trigger_task(
            Path("clear_notifications".to_string()),
            State(state.clone()),
        )
        .await
        .expect("trigger task should redirect");
        assert_eq!(redirect.into_response().status(), StatusCode::SEE_OTHER);

        let conn = state.db.lock().expect("lock sqlite");
        let runs = db::list_task_runs(&conn, "clear_notifications", 10).expect("list task runs");
        assert_eq!(runs.len(), 1);
    }

    #[tokio::test]
    async fn dash_create_task_redirects_and_persists_user_task() {
        let state = build_state();

        let redirect = dash_create_task(
            State(state.clone()),
            Form(TaskFormInput {
                code: "user_task".to_string(),
                name: "User Task".to_string(),
                description: "created from test".to_string(),
                schedule: "@hourly".to_string(),
                enabled: Some("1".to_string()),
                kind: Some("1".to_string()),
            }),
        )
        .await
        .expect("create task should redirect");
        assert_eq!(redirect.into_response().status(), StatusCode::SEE_OTHER);

        let conn = state.db.lock().expect("lock sqlite");
        let task = db::get_task_by_code(&conn, "user_task")
            .expect("query created task")
            .expect("task exists");
        assert_eq!(task.name, "User Task");
        assert_eq!(task.kind, TASK_KIND_USER);
    }

    #[tokio::test]
    async fn dash_update_task_keeps_code_and_updates_fields() {
        let state = build_state();
        let task_id = {
            let conn = state.db.lock().expect("lock sqlite");
            db::create_task(
                &conn,
                &db::TaskMutation {
                    code: "user_task".to_string(),
                    name: "User Task".to_string(),
                    description: "created from test".to_string(),
                    schedule: "@hourly".to_string(),
                    enabled: 1,
                    kind: TASK_KIND_USER,
                },
            )
            .expect("create task")
        };

        let redirect = dash_update_task(
            Path(task_id),
            State(state.clone()),
            Form(TaskFormInput {
                code: "changed".to_string(),
                name: "Renamed Task".to_string(),
                description: "updated".to_string(),
                schedule: "@daily".to_string(),
                enabled: None,
                kind: Some("1".to_string()),
            }),
        )
        .await
        .expect("update task should redirect");
        assert_eq!(redirect.into_response().status(), StatusCode::SEE_OTHER);

        let conn = state.db.lock().expect("lock sqlite");
        let task = db::get_task_by_id(&conn, task_id)
            .expect("query updated task")
            .expect("task exists");
        assert_eq!(task.code, "user_task");
        assert_eq!(task.name, "Renamed Task");
        assert_eq!(task.schedule, "@daily");
        assert_eq!(task.enabled, 0);
    }

    #[tokio::test]
    async fn dash_delete_task_soft_deletes_user_task() {
        let state = build_state();
        let task_id = {
            let conn = state.db.lock().expect("lock sqlite");
            db::create_task(
                &conn,
                &db::TaskMutation {
                    code: "user_task".to_string(),
                    name: "User Task".to_string(),
                    description: "created from test".to_string(),
                    schedule: "@hourly".to_string(),
                    enabled: 1,
                    kind: TASK_KIND_USER,
                },
            )
            .expect("create task")
        };

        let redirect = dash_delete_task(Path(task_id), State(state.clone()))
            .await
            .expect("delete task should redirect");
        assert_eq!(redirect.into_response().status(), StatusCode::SEE_OTHER);

        let conn = state.db.lock().expect("lock sqlite");
        assert!(db::get_task_by_id(&conn, task_id)
            .expect("query deleted task")
            .is_none());
    }

    #[tokio::test]
    async fn dash_delete_task_rejects_internal_task() {
        let state = build_state();
        let task_id = {
            let conn = state.db.lock().expect("lock sqlite");
            db::create_task(
                &conn,
                &db::TaskMutation {
                    code: "internal_task".to_string(),
                    name: "Internal Task".to_string(),
                    description: "created from test".to_string(),
                    schedule: "@hourly".to_string(),
                    enabled: 1,
                    kind: TASK_KIND_INTERNAL,
                },
            )
            .expect("create task")
        };

        let err = dash_delete_task(Path(task_id), State(state.clone()))
            .await
            .expect_err("internal task delete should fail");
        assert_eq!(err.into_response().status(), StatusCode::BAD_REQUEST);

        let conn = state.db.lock().expect("lock sqlite");
        assert!(db::get_task_by_id(&conn, task_id)
            .expect("query internal task")
            .is_some());
    }
}
