use std::sync::{Arc, OnceLock};

use anyhow::Result;
use tokio::sync::Mutex;
use tokio_cron_scheduler::{Job, JobScheduler};
use tracing::{error, info};

use crate::{db, web::AppState};

static SCHEDULER: OnceLock<Mutex<JobScheduler>> = OnceLock::new();

pub async fn start(state: Arc<AppState>) -> Result<()> {
    if SCHEDULER.get().is_some() {
        return Ok(());
    }

    let scheduler = JobScheduler::new().await?;
    let state_for_job = state.clone();
    let job = Job::new_async("1/30 * * * * *", move |_uuid, _l| {
        let state = state_for_job.clone();
        Box::pin(async move {
            match state.db.lock() {
                Ok(conn) => {
                    if let Err(err) =
                        db::record_task_run(&conn, "heartbeat", "ok", "scheduler tick")
                    {
                        error!(error = %err, "failed to record task run");
                    }
                }
                Err(err) => {
                    error!(error = %err, "db mutex poisoned");
                }
            }
        })
    })?;

    scheduler.add(job).await?;
    scheduler.start().await?;
    info!("job scheduler started");

    let _ = SCHEDULER.set(Mutex::new(scheduler));
    Ok(())
}

pub async fn stop() {
    if let Some(scheduler) = SCHEDULER.get() {
        let mut scheduler = scheduler.lock().await;
        let _ = scheduler.shutdown().await;
        info!("job scheduler stopped");
    }
}
