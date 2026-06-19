use std::net::SocketAddr;
use std::sync::Arc;

use anyhow::Result;
use tokio::signal;
use tracing::{error, info};

use crate::{cache, db, jobs, web};

#[derive(Clone)]
pub struct Runtime {
    sqlite_file: String,
    state: Arc<web::AppState>,
}

impl Runtime {
    pub fn new(sqlite_file: String) -> Result<Self> {
        let cache_paths = cache::RuntimeCachePaths::from_sqlite(&sqlite_file)?;
        std::fs::create_dir_all(&cache_paths.root)?;
        std::fs::create_dir_all(&cache_paths.updater_root)?;

        let conn = db::open_and_init(&sqlite_file)?;
        let state = Arc::new(web::AppState::new(sqlite_file.clone(), cache_paths, conn));

        Ok(Self { sqlite_file, state })
    }

    pub async fn run_supervisor(&self, listen_addr: SocketAddr, max_failures: i32) -> Result<()> {
        let mut failures = 0;

        loop {
            info!(%listen_addr, failures, "supervisor launching worker");
            let result = self.run_worker(listen_addr).await;
            match result {
                Ok(()) => return Ok(()),
                Err(err) => {
                    failures += 1;
                    error!(error = %err, failures, "worker exited with error");
                    if max_failures > 0 && failures >= max_failures {
                        return Err(err);
                    }
                }
            }
        }
    }

    pub async fn run_worker(&self, listen_addr: SocketAddr) -> Result<()> {
        jobs::start(self.state.clone()).await?;

        let app = web::router(self.state.clone());
        let listener = tokio::net::TcpListener::bind(listen_addr).await?;
        info!(%listen_addr, sqlite = %self.sqlite_file, "worker listening");

        axum::serve(listener, app)
            .with_graceful_shutdown(async {
                let _ = signal::ctrl_c().await;
                info!("shutdown signal received");
            })
            .await?;

        jobs::stop().await;
        Ok(())
    }
}
