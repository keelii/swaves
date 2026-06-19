use std::fs;
use std::path::{Path, PathBuf};

use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};

use crate::{Error, Result};

pub const RUNTIME_INFO_FILE_NAME: &str = "master-runtime.json";
pub const RESTART_REQUEST_FILE_NAME: &str = "restart-request.json";
pub const UPDATER_DIR_NAME: &str = "updater";

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RuntimeLayout {
    cache_root: PathBuf,
    runtime_info_path: PathBuf,
    restart_request_path: PathBuf,
    updater_dir: PathBuf,
}

impl RuntimeLayout {
    pub fn new(cache_root: impl Into<PathBuf>) -> Self {
        let cache_root = cache_root.into();
        Self {
            runtime_info_path: cache_root.join(RUNTIME_INFO_FILE_NAME),
            restart_request_path: cache_root.join(RESTART_REQUEST_FILE_NAME),
            updater_dir: cache_root.join(UPDATER_DIR_NAME),
            cache_root,
        }
    }

    pub fn validate(&self) -> Result<()> {
        if self.cache_root.as_os_str().is_empty() {
            return Err(Error::MissingCacheRoot);
        }
        Ok(())
    }

    pub fn ensure_dirs(&self) -> Result<()> {
        self.validate()?;
        fs::create_dir_all(&self.cache_root)?;
        fs::create_dir_all(&self.updater_dir)?;
        Ok(())
    }

    pub fn cache_root(&self) -> &Path {
        &self.cache_root
    }

    pub fn runtime_info_path(&self) -> &Path {
        &self.runtime_info_path
    }

    pub fn restart_request_path(&self) -> &Path {
        &self.restart_request_path
    }

    pub fn updater_dir(&self) -> &Path {
        &self.updater_dir
    }

    pub fn write_runtime_info(&self, info: &RuntimeInfo) -> Result<()> {
        self.write_json(self.runtime_info_path(), info)
    }

    pub fn read_runtime_info(&self) -> Result<RuntimeInfo> {
        self.read_json(self.runtime_info_path())
    }

    pub fn write_restart_request(&self, request: &RestartRequest) -> Result<()> {
        self.write_json(self.restart_request_path(), request)
    }

    pub fn read_restart_request(&self) -> Result<RestartRequest> {
        self.read_json(self.restart_request_path())
    }

    fn write_json<T>(&self, path: &Path, value: &T) -> Result<()>
    where
        T: Serialize,
    {
        self.ensure_dirs()?;
        let data = serde_json::to_vec_pretty(value)?;
        fs::write(path, data)?;
        Ok(())
    }

    fn read_json<T>(&self, path: &Path) -> Result<T>
    where
        T: DeserializeOwned,
    {
        self.validate()?;
        match fs::read(path) {
            Ok(data) => Ok(serde_json::from_slice(&data)?),
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
                Err(Error::MissingProtocolFile(path.to_path_buf()))
            }
            Err(err) => Err(err.into()),
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct RuntimeInfo {
    pub pid: u32,
    pub executable: PathBuf,
    pub args: Vec<String>,
    pub working_dir: Option<PathBuf>,
    pub version: Option<String>,
    pub updated_at_unix: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(tag = "kind", content = "value", rename_all = "snake_case")]
pub enum RestartReason {
    Upgrade,
    Manual,
    CrashRecovery,
    Custom(String),
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct RestartRequest {
    pub reason: RestartReason,
    pub requested_at_unix: i64,
    pub target_version: Option<String>,
    pub archive_name: Option<String>,
}

impl RestartRequest {
    pub fn upgrade(
        requested_at_unix: i64,
        target_version: impl Into<String>,
        archive_name: impl Into<String>,
    ) -> Self {
        Self {
            reason: RestartReason::Upgrade,
            requested_at_unix,
            target_version: Some(target_version.into()),
            archive_name: Some(archive_name.into()),
        }
    }
}

#[cfg(test)]
mod tests {
    use tempfile::tempdir;

    use super::{RestartReason, RestartRequest, RuntimeInfo, RuntimeLayout};

    #[test]
    fn layout_uses_updater_subdirectory_under_cache_root() {
        let layout = RuntimeLayout::new("/tmp/swaves-runtime");

        assert_eq!(
            layout.runtime_info_path(),
            std::path::Path::new("/tmp/swaves-runtime/master-runtime.json")
        );
        assert_eq!(
            layout.restart_request_path(),
            std::path::Path::new("/tmp/swaves-runtime/restart-request.json")
        );
        assert_eq!(
            layout.updater_dir(),
            std::path::Path::new("/tmp/swaves-runtime/updater")
        );
    }

    #[test]
    fn runtime_info_round_trips_through_protocol_file() {
        let dir = tempdir().unwrap();
        let layout = RuntimeLayout::new(dir.path());
        let info = RuntimeInfo {
            pid: 42,
            executable: "/opt/swaves/swaves".into(),
            args: vec!["swaves".into(), "--daemon".into()],
            working_dir: Some("/opt/swaves".into()),
            version: Some("v2.0.0".into()),
            updated_at_unix: 1_748_411_200,
        };

        layout.write_runtime_info(&info).unwrap();

        assert_eq!(layout.read_runtime_info().unwrap(), info);
    }

    #[test]
    fn restart_request_round_trips_through_protocol_file() {
        let dir = tempdir().unwrap();
        let layout = RuntimeLayout::new(dir.path());
        let request = RestartRequest {
            reason: RestartReason::Manual,
            requested_at_unix: 1_748_411_200,
            target_version: Some("v2.0.1".into()),
            archive_name: Some("swaves_v2.0.1_linux_amd64.tar.gz".into()),
        };

        layout.write_restart_request(&request).unwrap();

        assert_eq!(layout.read_restart_request().unwrap(), request);
    }
}
