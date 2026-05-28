use std::path::{Path, PathBuf};

use swaves_supervisor::{RestartRequest, RuntimeInfo, RuntimeLayout};

use crate::{Error, ReleaseAsset, Result};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct LocalArchive {
    version: String,
    archive_name: String,
    archive_path: PathBuf,
}

impl LocalArchive {
    pub fn new(
        version: impl Into<String>,
        archive_name: impl Into<String>,
        archive_path: impl Into<PathBuf>,
    ) -> Result<Self> {
        let version = version.into().trim().to_owned();
        if version.is_empty() {
            return Err(Error::MissingVersion);
        }

        let archive_name = archive_name.into().trim().to_owned();
        if archive_name.is_empty() {
            return Err(Error::MissingArchiveName);
        }

        let archive_path = archive_path.into();
        if archive_path.as_os_str().is_empty() {
            return Err(Error::MissingArchivePath);
        }

        Ok(Self {
            version,
            archive_name,
            archive_path,
        })
    }

    pub fn version(&self) -> &str {
        &self.version
    }

    pub fn archive_name(&self) -> &str {
        &self.archive_name
    }

    pub fn archive_path(&self) -> &Path {
        &self.archive_path
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum InstallSource {
    Release(ReleaseAsset),
    Local(LocalArchive),
}

impl InstallSource {
    fn version(&self) -> &str {
        match self {
            Self::Release(asset) => asset.version(),
            Self::Local(archive) => archive.version(),
        }
    }

    fn archive_name(&self) -> &str {
        match self {
            Self::Release(asset) => asset.archive_name(),
            Self::Local(archive) => archive.archive_name(),
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct InstallPlan {
    source: InstallSource,
    target_executable: PathBuf,
    staging_dir: PathBuf,
    restart_request: RestartRequest,
}

impl InstallPlan {
    pub fn for_active_runtime(
        layout: &RuntimeLayout,
        runtime: &RuntimeInfo,
        source: InstallSource,
        requested_at_unix: i64,
    ) -> Result<Self> {
        if runtime.executable.as_os_str().is_empty() {
            return Err(Error::MissingRuntimeExecutable);
        }

        Ok(Self {
            target_executable: runtime.executable.clone(),
            staging_dir: layout.updater_dir().to_path_buf(),
            restart_request: RestartRequest::upgrade(
                requested_at_unix,
                source.version(),
                source.archive_name(),
            ),
            source,
        })
    }

    pub fn source(&self) -> &InstallSource {
        &self.source
    }

    pub fn target_executable(&self) -> &Path {
        &self.target_executable
    }

    pub fn staging_dir(&self) -> &Path {
        &self.staging_dir
    }

    pub fn restart_request(&self) -> &RestartRequest {
        &self.restart_request
    }

    pub fn queue_restart(&self, layout: &RuntimeLayout) -> Result<()> {
        layout.write_restart_request(&self.restart_request)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use tempfile::tempdir;

    use swaves_supervisor::{RestartReason, RuntimeInfo, RuntimeLayout};

    use crate::{InstallPlan, InstallSource, LocalArchive, ReleaseAsset};

    fn runtime_info() -> RuntimeInfo {
        RuntimeInfo {
            pid: 9,
            executable: "/opt/swaves/swaves".into(),
            args: vec!["swaves".into()],
            working_dir: Some("/opt/swaves".into()),
            version: Some("v1.9.0".into()),
            updated_at_unix: 1_748_411_200,
        }
    }

    #[test]
    fn install_plan_for_release_uses_supervisor_protocol_and_updater_dir() {
        let dir = tempdir().unwrap();
        let layout = RuntimeLayout::new(dir.path());
        let source = InstallSource::Release(
            ReleaseAsset::new(
                "v2.0.0",
                "swaves_v2.0.0_linux_amd64.tar.gz",
                "https://example.invalid/swaves.tar.gz",
                Some("https://example.invalid/swaves.sha256".into()),
            )
            .unwrap(),
        );

        let plan = InstallPlan::for_active_runtime(&layout, &runtime_info(), source, 1_748_411_999)
            .unwrap();

        assert_eq!(plan.target_executable(), Path::new("/opt/swaves/swaves"));
        assert_eq!(plan.staging_dir(), layout.updater_dir());
        assert_eq!(plan.restart_request().reason, RestartReason::Upgrade);
        assert_eq!(
            plan.restart_request().target_version.as_deref(),
            Some("v2.0.0")
        );
        assert_eq!(
            plan.restart_request().archive_name.as_deref(),
            Some("swaves_v2.0.0_linux_amd64.tar.gz")
        );
    }

    #[test]
    fn install_plan_can_queue_restart_via_supervisor_layout() {
        let dir = tempdir().unwrap();
        let layout = RuntimeLayout::new(dir.path());
        let source = InstallSource::Local(
            LocalArchive::new(
                "v2.0.1",
                "swaves_v2.0.1_linux_amd64.tar.gz",
                "/tmp/swaves_v2.0.1_linux_amd64.tar.gz",
            )
            .unwrap(),
        );
        let plan = InstallPlan::for_active_runtime(&layout, &runtime_info(), source, 1_748_412_000)
            .unwrap();

        plan.queue_restart(&layout).unwrap();

        assert_eq!(
            layout.read_restart_request().unwrap(),
            plan.restart_request().clone()
        );
    }
}
