use std::path::Path;

use crate::{RestartRequest, Result, RuntimeInfo, RuntimeLayout, SupervisorConfig};

#[derive(Clone, Debug)]
pub struct SupervisorRuntime {
    config: SupervisorConfig,
}

impl SupervisorRuntime {
    pub fn new(config: SupervisorConfig) -> Result<Self> {
        config.validate()?;
        Ok(Self { config })
    }

    pub fn config(&self) -> &SupervisorConfig {
        &self.config
    }

    pub fn layout(&self) -> &RuntimeLayout {
        &self.config.runtime_layout
    }

    pub fn worker_command(&self) -> &Path {
        self.config.worker_command()
    }

    pub fn publish_runtime_info(&self, info: &RuntimeInfo) -> Result<()> {
        self.layout().write_runtime_info(info)
    }

    pub fn queue_restart(&self, request: &RestartRequest) -> Result<()> {
        self.layout().write_restart_request(request)
    }
}
