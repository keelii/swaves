use std::path::PathBuf;

use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("supervisor cache root is required")]
    MissingCacheRoot,
    #[error("listen address is required")]
    MissingListenAddr,
    #[error("worker command is required")]
    MissingWorkerCommand,
    #[error("protocol file not found: {0}")]
    MissingProtocolFile(PathBuf),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
    #[error("json error: {0}")]
    Json(#[from] serde_json::Error),
}

pub type Result<T> = std::result::Result<T, Error>;
