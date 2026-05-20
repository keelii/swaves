mod app;
mod cache;
mod db;
mod htmlutil;
mod jobs;
mod markdown;
mod routes;
mod view;
mod web;

use std::net::SocketAddr;

use anyhow::Result;
use clap::Parser;
use tracing::info;

#[derive(Parser, Debug, Clone)]
#[command(
    name = "swaves-rs",
    about = "Experimental full-replacement Rust POC for swaves"
)]
struct Cli {
    /// SQLite file path (same semantic target as Go runtime)
    sqlite_file: String,

    /// Listen address
    #[arg(long, default_value = "127.0.0.1:4096")]
    listen_addr: SocketAddr,

    /// Enable daemon/supervisor mode for this POC run
    #[arg(long, default_value_t = true)]
    daemon_mode: bool,

    /// Max consecutive worker failures before supervisor exits
    #[arg(long, default_value_t = 5)]
    max_failures: i32,

    /// Run worker path directly
    #[arg(long, default_value_t = false)]
    worker: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_target(false)
        .compact()
        .init();

    let cli = Cli::parse();
    let runtime = app::Runtime::new(cli.sqlite_file.clone())?;

    if cli.worker || !cli.daemon_mode {
        info!("starting worker mode");
        runtime.run_worker(cli.listen_addr).await?;
        return Ok(());
    }

    info!("starting supervisor mode");
    runtime
        .run_supervisor(cli.listen_addr, cli.max_failures)
        .await?;
    Ok(())
}
