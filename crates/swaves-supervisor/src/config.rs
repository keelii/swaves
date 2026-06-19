use std::path::{Path, PathBuf};

use crate::{Error, Result, RuntimeLayout};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SupervisorConfig {
    pub listen_addr: String,
    pub worker_command: PathBuf,
    pub max_failures: u32,
    pub ready_timeout_secs: u64,
    pub shutdown_timeout_secs: u64,
    pub drain_timeout_secs: u64,
    pub runtime_layout: RuntimeLayout,
}

impl SupervisorConfig {
    pub fn validate(&self) -> Result<()> {
        if self.listen_addr.trim().is_empty() {
            return Err(Error::MissingListenAddr);
        }
        if self.worker_command.as_os_str().is_empty() {
            return Err(Error::MissingWorkerCommand);
        }
        self.runtime_layout.validate()
    }

    pub fn worker_command(&self) -> &Path {
        &self.worker_command
    }
}
