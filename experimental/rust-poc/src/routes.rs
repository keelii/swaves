use std::collections::BTreeMap;

use axum::extract::State;
use axum::http::StatusCode;
use axum::response::{Html, IntoResponse};
use axum::routing::get;
use axum::Router;
use minijinja::context;
use serde::Serialize;

use crate::app::AppState;

#[derive(Clone, Debug, Default)]
pub struct NamedRoutes {
    routes: BTreeMap<&'static str, &'static str>,
}

impl NamedRoutes {
    pub fn insert(&mut self, name: &'static str, path: &'static str) {
        self.routes.insert(name, path);
    }

    pub fn url_for(
        &self,
        name: &str,
        params: &BTreeMap<String, String>,
        query: &BTreeMap<String, String>,
    ) -> Result<String, String> {
        let pattern = self
            .routes
            .get(name)
            .copied()
            .ok_or_else(|| format!("unknown route: {name}"))?;
        let mut rendered = pattern.to_string();
        for (key, value) in params {
            rendered = rendered.replace(&format!("{{{key}}}"), value);
        }
        if rendered.contains('{') || rendered.contains('}') {
            return Err(format!("missing route params for {name}"));
        }
        if query.is_empty() {
            return Ok(rendered);
        }
        let query_string = query
            .iter()
            .map(|(key, value)| format!("{key}={value}"))
            .collect::<Vec<_>>()
            .join("&");
        Ok(format!("{rendered}?{query_string}"))
    }
}

pub fn named_routes() -> NamedRoutes {
    let mut routes = NamedRoutes::default();
    routes.insert("dash.login.submit", "/dash/login");
    routes.insert("dash.notifications.api.delete", "/dash/api/notifications/delete");
    routes.insert("dash.notifications.api.read", "/dash/api/notifications/read");
    routes.insert("dash.notifications.api.read_all", "/dash/api/notifications/read_all");
    routes.insert(
        "dash.notifications.api.unread_count",
        "/dash/api/notifications/unread_count",
    );
    routes.insert(
        "dash.settings.api.ui_state.update",
        "/dash/api/settings/ui-state",
    );
    routes.insert("dash.posts.edit", "/dash/posts/{id}/edit");
    routes.insert("site.action.like", "/_action/like/{postID}");
    routes
}

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/api/template-probe", get(template_probe))
        .route("/dash/login", get(dash_login))
        .route("/posts/demo", get(site_post_demo))
        .with_state(state)
}

async fn template_probe(State(state): State<AppState>) -> impl IntoResponse {
    match state.view.render_str(
        r#"{% include "include/favicon.html" %}
{{ Settings("author") }}|{{ UrlFor("dash.login.submit") }}|{{ StaticUrl("/static/site/style.css") }}|{{ 1710131696 | articleTime }}|{{ GetBasePath() }}"#,
        context! {
            RouteName => "template.probe",
            Title => "template probe",
            _csrf_token_value => "probe-token",
        },
    ) {
        Ok(rendered) => Html(rendered).into_response(),
        Err(err) => (StatusCode::INTERNAL_SERVER_ERROR, err).into_response(),
    }
}

async fn dash_login(State(state): State<AppState>) -> impl IntoResponse {
    match state.view.render(
        "dash/dash_login.html",
        context! {
            Error => Option::<String>::None,
            IsLogin => false,
            IsMobile => false,
            NotificationUnreadCount => 0,
            ReturnUrl => "/dash/posts",
            RouteName => "dash.login.show",
            Title => "Dash Login",
            _csrf_token_value => "stage0-login-token",
        },
    ) {
        Ok(rendered) => Html(rendered).into_response(),
        Err(err) => (StatusCode::INTERNAL_SERVER_ERROR, err).into_response(),
    }
}

async fn site_post_demo(State(state): State<AppState>) -> impl IntoResponse {
    let post = DemoPost::default();
    match state.view.render(
        "themes/tuft/post.html",
        context! {
            CanonicalURL => "https://example.com/posts/demo",
            CommentCount => 0,
            IsLogin => true,
            IsMobile => false,
            LikeCount => 3,
            Liked => false,
            MetaDescription => "rust migration stage-0 article render",
            MetaKeywords => "swaves,rust,poc",
            Post => post,
            ReadUV => 12,
            RouteName => "site.post.detail",
            Title => "Rust Stage-0 Demo Post",
            UrlPath => "/posts/demo",
        },
    ) {
        Ok(rendered) => Html(rendered).into_response(),
        Err(err) => (StatusCode::INTERNAL_SERVER_ERROR, err).into_response(),
    }
}

#[derive(Serialize)]
struct DemoPost {
    #[serde(rename = "Category")]
    category: Option<DemoLinkItem>,
    #[serde(rename = "CommentEnabled")]
    comment_enabled: i32,
    #[serde(rename = "HTML")]
    html: &'static str,
    #[serde(rename = "ID")]
    id: i64,
    #[serde(rename = "Kind")]
    kind: i32,
    #[serde(rename = "Next")]
    next: Option<DemoNavPost>,
    #[serde(rename = "Prev")]
    prev: Option<DemoNavPost>,
    #[serde(rename = "PublishedAt")]
    published_at: i64,
    #[serde(rename = "TOCHTML")]
    toc_html: &'static str,
    #[serde(rename = "Tags")]
    tags: Vec<DemoLinkItem>,
    #[serde(rename = "Title")]
    title: &'static str,
}

impl Default for DemoPost {
    fn default() -> Self {
        Self {
            category: None,
            comment_enabled: 0,
            html: "<p>Rust stage-0 baseline can render an existing site post template.</p>",
            id: 42,
            kind: 1,
            next: None,
            prev: None,
            published_at: 1_710_131_696,
            toc_html: "<ol><li><a href=\"#baseline\">Baseline</a></li></ol>",
            tags: Vec::new(),
            title: "Rust Stage-0 Demo Post",
        }
    }
}

#[derive(Serialize)]
struct DemoLinkItem {
    #[serde(rename = "Name")]
    name: String,
    #[serde(rename = "PermLink")]
    perm_link: String,
}

#[derive(Serialize)]
struct DemoNavPost {
    #[serde(rename = "PermLink")]
    perm_link: String,
    #[serde(rename = "Title")]
    title: String,
}

#[cfg(test)]
mod tests {
    use axum::body::Body;
    use axum::http::{Request, StatusCode};
    use tower::ServiceExt;

    use super::*;

    #[tokio::test]
    async fn template_probe_route_renders_existing_helpers() {
        let state = AppState::bootstrap().expect("bootstrap state");
        let app = router(state);
        let response = app
            .oneshot(
                Request::builder()
                    .uri("/api/template-probe")
                    .body(Body::empty())
                    .expect("build request"),
            )
            .await
            .expect("route response");

        assert_eq!(response.status(), StatusCode::OK);
    }
}
