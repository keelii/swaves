use std::sync::{Arc, OnceLock};

use anyhow::Result;
use tokio::sync::Mutex;
use tokio_cron_scheduler::{Job, JobScheduler};
use tracing::{error, info, warn};

use crate::{
    db::{self, TaskDefinition},
    web::AppState,
};

static SCHEDULER: OnceLock<Mutex<Option<SchedulerRuntime>>> = OnceLock::new();

const BUILTIN_TASKS: [TaskDefinition; 5] = [
    TaskDefinition {
        code: "database_backup",
        name: "数据备份",
        description: "定时备份数据库",
        schedule: "@daily",
        enabled: 1,
        kind: 0,
    },
    TaskDefinition {
        code: "clear_encrypted_posts",
        name: "清理过期加密文章",
        description: "定时清理加密文章",
        schedule: "@every 1m",
        enabled: 1,
        kind: 1,
    },
    TaskDefinition {
        code: "clear_notifications",
        name: "清理过期通知",
        description: "按保留天数清理过期通知",
        schedule: "@daily",
        enabled: 1,
        kind: 0,
    },
    TaskDefinition {
        code: "check_app_update",
        name: "检查应用更新",
        description: "每天检查 swaves 是否有新的稳定版本可升级",
        schedule: "@daily",
        enabled: 1,
        kind: 0,
    },
    TaskDefinition {
        code: "remote_backup_data",
        name: "远程备份数据",
        description: "备份数据库到远程",
        schedule: "@daily",
        enabled: 1,
        kind: 1,
    },
];

type JobFunc = fn() -> TaskExecution;

const REGISTERED_JOBS: [(&str, JobFunc); 5] = [
    ("database_backup", database_backup_job),
    ("clear_encrypted_posts", clear_encrypted_posts_job),
    ("clear_notifications", clear_notifications_job),
    ("check_app_update", check_app_update_job),
    ("remote_backup_data", remote_backup_data_job),
];

struct SchedulerRuntime {
    scheduler: JobScheduler,
    job_codes: Vec<String>,
}

#[derive(Debug, Clone)]
struct TaskExecution {
    status: &'static str,
    message: &'static str,
}

pub async fn start(state: Arc<AppState>) -> Result<()> {
    let scheduler_state = scheduler_state();
    let mut guard = scheduler_state.lock().await;
    if guard.is_some() {
        info!("job scheduler start skipped: already running");
        return Ok(());
    }

    {
        let conn = state
            .db
            .lock()
            .map_err(|err| anyhow::anyhow!("db mutex poisoned during task seeding: {err}"))?;
        for task in BUILTIN_TASKS.iter() {
            db::ensure_builtin_task(&conn, task)?;
        }
    }

    let scheduler = JobScheduler::new().await?;
    let enabled_tasks = {
        let conn = state
            .db
            .lock()
            .map_err(|err| anyhow::anyhow!("db mutex poisoned during task load: {err}"))?;
        db::list_enabled_task_records(&conn)?
    };

    let mut job_codes = Vec::new();
    for task in enabled_tasks {
        let Some(job_func) = registered_job(task.code.as_str()) else {
            warn!(task_code = %task.code, "skipping unregistered task code");
            continue;
        };
        let Some(schedule) = normalize_schedule(task.schedule.as_str()) else {
            warn!(task_code = %task.code, schedule = %task.schedule, "skipping invalid task schedule");
            continue;
        };

        let state_for_job = state.clone();
        let task_code = task.code.clone();
        let log_schedule = schedule.clone();
        let task_job = Job::new_async(schedule.as_str(), move |_uuid, _scheduler_lock| {
            let state = state_for_job.clone();
            let task_code = task_code.clone();
            Box::pin(async move {
                execute_task(state, task_code.as_str(), job_func).await;
            })
        })?;
        scheduler.add(task_job).await?;
        info!(task_code = %task.code, schedule = %log_schedule, "registered task schedule");
        job_codes.push(task.code);
    }

    scheduler.start().await?;
    info!(registered_tasks = job_codes.len(), "job scheduler started");
    *guard = Some(SchedulerRuntime {
        scheduler,
        job_codes,
    });
    Ok(())
}

pub async fn stop() {
    let scheduler_state = scheduler_state();
    let mut guard = scheduler_state.lock().await;
    let Some(mut runtime) = guard.take() else {
        info!("job scheduler stop skipped: not running");
        return;
    };
    drop(guard);

    let job_count = runtime.job_codes.len();
    let _ = runtime.scheduler.shutdown().await;
    info!(registered_tasks = job_count, "job scheduler stopped");
}

fn scheduler_state() -> &'static Mutex<Option<SchedulerRuntime>> {
    SCHEDULER.get_or_init(|| Mutex::new(None))
}

async fn execute_task(state: Arc<AppState>, task_code: &str, job_func: JobFunc) {
    let started_at = unix_now();
    let execution = job_func();
    match state.db.lock() {
        Ok(conn) => {
            if let Err(err) = db::update_task_status(&conn, task_code, execution.status, started_at)
            {
                error!(task_code, status = execution.status, error = %err, "failed to update task status");
            }
            if let Err(err) =
                db::record_task_run(&conn, task_code, execution.status, execution.message)
            {
                error!(task_code, status = execution.status, error = %err, "failed to record task run");
            }
        }
        Err(err) => {
            error!(task_code, error = %err, "db mutex poisoned");
        }
    }

    match execution.status {
        "success" => info!(
            task_code,
            message = execution.message,
            "task execution completed"
        ),
        _ => warn!(
            task_code,
            message = execution.message,
            "task execution reported placeholder status"
        ),
    }
}

fn registered_job(code: &str) -> Option<JobFunc> {
    REGISTERED_JOBS
        .iter()
        .find_map(|(registered_code, job)| (*registered_code == code).then_some(*job))
}

fn normalize_schedule(schedule: &str) -> Option<String> {
    let trimmed = schedule.trim();
    if trimmed.is_empty() {
        return None;
    }

    if let Some(interval) = trimmed.strip_prefix("@every ") {
        return normalize_every_schedule(interval.trim());
    }

    let named = match trimmed {
        "@yearly" | "@annually" => return Some("0 0 0 1 1 *".to_string()),
        "@monthly" => return Some("0 0 0 1 * *".to_string()),
        "@weekly" => return Some("0 0 0 * * 0".to_string()),
        "@daily" => return Some("0 0 0 * * *".to_string()),
        "@midnight" => return Some("0 0 0 * * *".to_string()),
        "@hourly" => return Some("0 0 * * * *".to_string()),
        value if value.starts_with('@') => return None,
        value => value,
    };

    let fields: Vec<_> = named.split_whitespace().collect();
    match fields.len() {
        5 => Some(format!("0 {named}")),
        6 | 7 => Some(named.to_string()),
        _ => None,
    }
}

fn normalize_every_schedule(interval: &str) -> Option<String> {
    let split_at = interval
        .find(|ch: char| !ch.is_ascii_digit())
        .filter(|index| *index > 0)?;
    let (number, unit) = interval.split_at(split_at);
    let value = number.parse::<u64>().ok()?;
    if value == 0 {
        return None;
    }

    match unit {
        "s" if value <= 59 => Some(format!("0/{value} * * * * *")),
        "m" if value <= 59 => Some(format!("0 */{value} * * * *")),
        "h" if value <= 23 => Some(format!("0 0 */{value} * * *")),
        "d" if value <= 31 => Some(format!("0 0 0 */{value} * *")),
        _ => None,
    }
}

fn database_backup_job() -> TaskExecution {
    TaskExecution {
        status: "error",
        message: "rust poc placeholder: database backup job is not implemented yet",
    }
}

fn clear_encrypted_posts_job() -> TaskExecution {
    TaskExecution {
        status: "error",
        message: "rust poc placeholder: clear encrypted posts job is not implemented yet",
    }
}

fn clear_notifications_job() -> TaskExecution {
    TaskExecution {
        status: "error",
        message: "rust poc placeholder: clear notifications job is not implemented yet",
    }
}

fn check_app_update_job() -> TaskExecution {
    TaskExecution {
        status: "error",
        message: "rust poc placeholder: check app update job is not implemented yet",
    }
}

fn remote_backup_data_job() -> TaskExecution {
    TaskExecution {
        status: "error",
        message: "rust poc placeholder: remote backup job is not implemented yet",
    }
}

fn unix_now() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("current time before unix epoch")
        .as_secs() as i64
}

#[cfg(test)]
mod tests {
    use super::*;
    use rusqlite::Connection;

    use crate::{cache::RuntimeCachePaths, db, web::AppState};

    fn build_state() -> Arc<AppState> {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(include_str!(concat!(env!("OUT_DIR"), "/initial_sql.sql")))
            .expect("initialize schema");
        Arc::new(AppState::new(
            "/tmp/swaves-poc-test.sqlite".to_string(),
            RuntimeCachePaths {
                root: "/tmp/swaves-poc-cache".into(),
                updater_root: "/tmp/swaves-poc-cache/updater".into(),
            },
            conn,
        ))
    }

    #[tokio::test]
    async fn scheduler_start_and_stop_are_idempotent() {
        let state = build_state();

        start(state.clone())
            .await
            .expect("start scheduler first time");
        start(state.clone())
            .await
            .expect("start scheduler second time");

        stop().await;
        stop().await;

        start(state).await.expect("restart scheduler after stop");
        stop().await;
    }

    #[tokio::test]
    async fn scheduler_seeds_builtin_tasks() {
        let state = build_state();

        start(state.clone()).await.expect("start scheduler");

        let conn = state.db.lock().expect("lock sqlite");
        for task in BUILTIN_TASKS.iter() {
            let count = db::count_tasks_by_code(&conn, task.code).expect("count seeded task");
            assert_eq!(
                count, 1,
                "expected builtin task {} to exist once",
                task.code
            );
        }
        drop(conn);

        stop().await;
    }

    #[test]
    fn normalize_schedule_matches_go_style_inputs() {
        assert_eq!(normalize_schedule("@daily").as_deref(), Some("0 0 0 * * *"));
        assert_eq!(
            normalize_schedule("@every 1m").as_deref(),
            Some("0 */1 * * * *")
        );
        assert_eq!(
            normalize_schedule("@every 30s").as_deref(),
            Some("0/30 * * * * *")
        );
        assert_eq!(
            normalize_schedule("0 */5 * * *").as_deref(),
            Some("0 0 */5 * * *")
        );
        assert_eq!(
            normalize_schedule("1/30 * * * * *").as_deref(),
            Some("1/30 * * * * *")
        );
        assert_eq!(normalize_schedule("@reboot"), None);
        assert_eq!(normalize_schedule(""), None);
    }
}
