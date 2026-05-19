use std::path::{Path, PathBuf};

use anyhow::{bail, Result};

#[derive(Debug, Clone)]
pub struct RuntimeCachePaths {
    pub root: PathBuf,
    pub updater_root: PathBuf,
}

impl RuntimeCachePaths {
    pub fn from_sqlite(sqlite_file: &str) -> Result<Self> {
        let sqlite = Path::new(sqlite_file);
        if sqlite_file.trim().is_empty() {
            bail!("sqlite file is required");
        }
        let abs = if sqlite.is_absolute() {
            sqlite.to_path_buf()
        } else {
            std::env::current_dir()?.join(sqlite)
        };
        let parent = abs
            .parent()
            .ok_or_else(|| anyhow::anyhow!("sqlite parent directory is required"))?;
        let root = parent.join(".cache");
        let updater_root = root.join("updater");
        Ok(Self { root, updater_root })
    }
}
