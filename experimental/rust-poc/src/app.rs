use std::collections::BTreeMap;
use std::env;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use axum::Router;

use crate::db;
use crate::routes;
use crate::view::ViewEngine;

#[derive(Clone)]
pub struct AppState {
    pub listen_addr: String,
    pub view: Arc<ViewEngine>,
}

impl AppState {
    pub fn bootstrap() -> Result<Self, Box<dyn std::error::Error>> {
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let repo_root = manifest_dir
            .ancestors()
            .nth(2)
            .map(Path::to_path_buf)
            .ok_or("resolve repository root failed")?;
        let template_root = repo_root.join("web/templates");
        let sqlite_dsn = env::var("SWAVES_POC_SQLITE").unwrap_or_else(|_| ":memory:".to_string());
        db::open(&sqlite_dsn)?;

        let routes = Arc::new(routes::named_routes());
        let settings = Arc::new(default_settings());
        let view = Arc::new(ViewEngine::new(template_root, settings, routes.clone()));

        Ok(Self {
            listen_addr: env::var("SWAVES_POC_ADDR").unwrap_or_else(|_| "127.0.0.1:4300".to_string()),
            view,
        })
    }
}

pub fn build_router(state: AppState) -> Router {
    routes::router(state)
}

fn default_settings() -> BTreeMap<String, String> {
    BTreeMap::from([
        ("author".to_string(), "keelii".to_string()),
        ("language".to_string(), "zh-CN".to_string()),
        ("mode".to_string(), "light".to_string()),
        ("site_copyright".to_string(), "@copyright {{year}} keelii".to_string()),
        ("site_desc".to_string(), "swaves rust migration stage-0 baseline".to_string()),
        ("site_keywords".to_string(), "swaves,rust,poc".to_string()),
        ("site_name".to_string(), "swaves".to_string()),
    ])
}
