use std::borrow::Cow;
use std::collections::BTreeMap;
use std::path::{Component, Path, PathBuf};
use std::sync::Arc;

use chrono::{Local, TimeZone};
use minijinja::value::{Kwargs, Value};
use minijinja::{Environment, Error, ErrorKind, State, UndefinedBehavior};
use serde::Serialize;

use crate::routes::NamedRoutes;

#[derive(Clone)]
pub struct ViewEngine {
    template_root: PathBuf,
    settings: Arc<BTreeMap<String, String>>,
    routes: Arc<NamedRoutes>,
}

impl ViewEngine {
    pub fn new(
        template_root: PathBuf,
        settings: Arc<BTreeMap<String, String>>,
        routes: Arc<NamedRoutes>,
    ) -> Self {
        Self {
            template_root,
            settings,
            routes,
        }
    }

    pub fn render<S: Serialize>(&self, name: &str, context: S) -> Result<String, String> {
        let env = self.build_env().map_err(|err| err.to_string())?;
        let template_name = normalize_template_name(name).map_err(|err| err.to_string())?;
        let template = env
            .get_template(&template_name)
            .map_err(|err| err.to_string())?;
        template.render(context).map_err(|err| err.to_string())
    }

    pub fn render_str<S: Serialize>(&self, source: &str, context: S) -> Result<String, String> {
        let env = self.build_env().map_err(|err| err.to_string())?;
        env.render_str(source, context).map_err(|err| err.to_string())
    }

    fn build_env(&self) -> Result<Environment<'static>, Error> {
        let mut env = Environment::new();
        env.set_undefined_behavior(UndefinedBehavior::Lenient);
        let template_root = self
            .template_root
            .canonicalize()
            .map_err(|err| Error::new(ErrorKind::InvalidOperation, err.to_string()))?;
        env.set_loader(Self::loader(template_root));
        env.set_path_join_callback(resolve_template_import_path);
        register_globals(&mut env);
        register_filters(&mut env);
        register_functions(&mut env, self.settings.clone(), self.routes.clone());
        Ok(env)
    }

    fn loader(template_root: PathBuf) -> impl Fn(&str) -> Result<Option<String>, Error> + Send + Sync + 'static {
        move |name: &str| {
            let normalized_name = normalize_template_name(name)?;
            let path = template_root.join(&normalized_name);
            let canonical = path.canonicalize().map_err(|err| {
                if err.kind() == std::io::ErrorKind::NotFound {
                    Error::new(ErrorKind::TemplateNotFound, normalized_name.clone())
                } else {
                    Error::new(ErrorKind::InvalidOperation, err.to_string())
                }
            })?;
            if !canonical.starts_with(&template_root) {
                return Err(Error::new(
                    ErrorKind::InvalidOperation,
                    format!("template path escapes root: {normalized_name}"),
                ));
            }
            std::fs::read_to_string(canonical)
                .map(Some)
                .map_err(|err| Error::new(ErrorKind::InvalidOperation, err.to_string()))
        }
    }
}

fn register_globals(env: &mut Environment<'static>) {
    env.add_function("_csrf_token", |state: &State| -> Value {
        let token = state
            .lookup("_csrf_token_value")
            .and_then(|value| value.as_str().map(str::to_owned))
            .unwrap_or_default();
        if token.is_empty() {
            return Value::from_safe_string(String::new());
        }
        Value::from_safe_string(format!(
            r#"<input type="hidden" name="_csrf_token" value="{token}">"#
        ))
    });
}

fn register_filters(env: &mut Environment<'static>) {
    env.add_filter("articleTime", |value: Value| -> String {
        let Some(ts) = value.as_i64() else {
            return "-".to_string();
        };
        match Local.timestamp_opt(ts, 0).single() {
            Some(dt) => dt.format("%Y-%m-%d %H:%M").to_string(),
            None => "-".to_string(),
        }
    });
    env.add_filter("datetimeReplacer", |value: Value| -> String {
        value
            .as_str()
            .unwrap_or_default()
            .replace("{{year}}", &Local::now().format("%Y").to_string())
    });
}

fn register_functions(
    env: &mut Environment<'static>,
    settings: Arc<BTreeMap<String, String>>,
    routes: Arc<NamedRoutes>,
) {
    env.add_function("GetBasePath", || -> String { "/".to_string() });

    env.add_function("LucideIcon", |name: String, size: Option<u32>| -> Value {
        let size = size.unwrap_or(16);
        Value::from_safe_string(format!(
            r#"<svg data-name="{name}" width="{size}" height="{size}" aria-hidden="true"></svg>"#
        ))
    });

    env.add_function("Settings", move |key: String| -> String {
        settings.get(key.trim()).cloned().unwrap_or_default()
    });

    env.add_function("StaticUrl", |path: String| -> String { path });

    env.add_function("UrlFor", move |name: String, kwargs: Kwargs| -> Result<String, Error> {
        let mut params = BTreeMap::new();
        for key in kwargs.args() {
            let value = kwargs
                .get::<Value>(key)
                .map_err(|err| Error::new(ErrorKind::InvalidOperation, err.to_string()))?;
            params.insert(key.to_string(), stringify_value(&value));
        }
        routes
            .url_for(name.trim(), &params, &BTreeMap::new())
            .map_err(|err| Error::new(ErrorKind::InvalidOperation, err))
    });
}

fn stringify_value(value: &Value) -> String {
    if let Some(text) = value.as_str() {
        return text.to_string();
    }
    if let Some(number) = value.as_i64() {
        return number.to_string();
    }
    value.to_string()
}

fn normalize_template_name(name: &str) -> Result<String, Error> {
    let candidate = name.trim();
    if candidate.is_empty() {
        return Err(Error::new(
            ErrorKind::InvalidOperation,
            "template name is required",
        ));
    }
    let normalized = Path::new(candidate)
        .components()
        .try_fold(PathBuf::new(), |mut acc, component| match component {
            Component::Normal(part) => {
                acc.push(part);
                Ok(acc)
            }
            Component::CurDir => Ok(acc),
            _ => Err(Error::new(
                ErrorKind::InvalidOperation,
                format!("invalid template path: {candidate}"),
            )),
        })?;
    let rendered = normalized.to_string_lossy().replace('\\', "/");
    if rendered.is_empty() {
        return Err(Error::new(
            ErrorKind::InvalidOperation,
            "template name is required",
        ));
    }
    Ok(rendered)
}

fn resolve_template_import_path<'a>(name: &'a str, parent: &'a str) -> Cow<'a, str> {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return Cow::Borrowed("");
    }
    if let Some(stripped) = trimmed.strip_prefix('/') {
        return Cow::Owned(clean_path(stripped));
    }
    if matches!(trimmed, "." | "..") || trimmed.starts_with("./") || trimmed.starts_with("../") {
        let base = Path::new(parent)
            .parent()
            .map(Path::to_path_buf)
            .unwrap_or_default();
        return Cow::Owned(clean_path(base.join(trimmed).to_string_lossy().as_ref()));
    }
    if trimmed.contains('/') {
        return Cow::Owned(clean_path(trimmed));
    }
    let base = Path::new(parent)
        .parent()
        .map(Path::to_path_buf)
        .unwrap_or_default();
    Cow::Owned(clean_path(base.join(trimmed).to_string_lossy().as_ref()))
}

fn clean_path(raw: &str) -> String {
    let mut path = PathBuf::new();
    for component in Path::new(raw).components() {
        match component {
            Component::CurDir => {}
            Component::Normal(part) => path.push(part),
            Component::ParentDir => {
                path.pop();
            }
            _ => {}
        }
    }
    path.to_string_lossy().replace('\\', "/")
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::sync::Arc;

    use minijinja::context;

    use super::*;
    use crate::routes::named_routes;

    fn engine() -> ViewEngine {
        let template_root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../web/templates");
        ViewEngine::new(
            template_root,
            Arc::new(BTreeMap::from([
                ("author".to_string(), "keelii".to_string()),
                ("language".to_string(), "zh-CN".to_string()),
                ("mode".to_string(), "light".to_string()),
                ("site_copyright".to_string(), "@copyright {{year}} keelii".to_string()),
                ("site_desc".to_string(), "stage-0 desc".to_string()),
                ("site_keywords".to_string(), "stage-0,keywords".to_string()),
                ("site_name".to_string(), "swaves".to_string()),
            ])),
            Arc::new(named_routes()),
        )
    }

    #[test]
    fn path_join_matches_repo_rules() {
        assert_eq!(
            resolve_template_import_path("../include/favicon.html", "dash/layout/base.html"),
            "dash/include/favicon.html"
        );
        assert_eq!(
            resolve_template_import_path("/include/favicon.html", "dash/layout/base.html"),
            "include/favicon.html"
        );
        assert_eq!(
            resolve_template_import_path("inc_meta.html", "themes/tuft/layout_main.html"),
            "themes/tuft/inc_meta.html"
        );
    }

    #[test]
    fn renders_existing_dash_login_template() {
        let rendered = engine()
            .render(
                "dash/dash_login.html",
                context! {
                    Error => Option::<String>::None,
                    IsLogin => false,
                    IsMobile => false,
                    NotificationUnreadCount => 0,
                    ReturnUrl => "/dash/posts",
                    RouteName => "dash.login.show",
                    Title => "Dash Login",
                    _csrf_token_value => "csrf-token",
                },
            )
            .expect("render dash login");

        assert!(rendered.contains("登录管理后台"));
        assert!(rendered.contains("&#x2f;dash&#x2f;login"), "{rendered}");
        assert!(rendered.contains("csrf-token"));
        assert!(rendered.contains("keelii"));
    }

    #[test]
    fn renders_existing_site_post_template() {
        let rendered = engine()
            .render(
                "themes/tuft/post.html",
                context! {
                    CanonicalURL => "https://example.com/posts/demo",
                    CommentCount => 0,
                    IsLogin => true,
                    IsMobile => false,
                    LikeCount => 2,
                    Liked => false,
                    MetaDescription => "desc",
                    MetaKeywords => "keywords",
                    Post => context! {
                        Category => Option::<String>::None,
                        CommentEnabled => 0,
                        HTML => "<p>Hello from Rust.</p>",
                        ID => 7,
                        Kind => 1,
                        Next => Option::<String>::None,
                        Prev => Option::<String>::None,
                        PublishedAt => 1_710_131_696,
                        TOCHTML => "<ol><li>Intro</li></ol>",
                        Tags => Vec::<String>::new(),
                        Title => "Rust Demo Post",
                    },
                    ReadUV => 9,
                    RouteName => "site.post.detail",
                    Title => "Rust Demo Post",
                    UrlPath => "/posts/demo",
                },
            )
            .expect("render site post");

        assert!(rendered.contains("Rust Demo Post"));
        assert!(rendered.contains("&#x2f;_action&#x2f;like&#x2f;7"), "{rendered}");
        assert!(rendered.contains("&#x2f;dash&#x2f;posts&#x2f;7&#x2f;edit"), "{rendered}");
        assert!(rendered.contains("Hello from Rust."));
    }
}
