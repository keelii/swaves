use anyhow::Result;
use rusqlite::{Connection, OptionalExtension};
use serde::Serialize;

const INITIAL_SQL: &str = include_str!(concat!(env!("OUT_DIR"), "/initial_sql.sql"));

pub const TASK_KIND_INTERNAL: i64 = 0;
pub const TASK_KIND_USER: i64 = 1;

pub fn open_and_init(sqlite_file: &str) -> Result<Connection> {
    let conn = Connection::open(sqlite_file)?;
    conn.execute_batch(INITIAL_SQL)?;
    Ok(conn)
}

pub fn record_task_run(
    conn: &Connection,
    task_code: &str,
    status: &str,
    message: &str,
) -> Result<()> {
    let now = unix_now();
    conn.execute(
        "INSERT INTO t_task_runs(task_code, status, message, started_at, finished_at, duration, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
        (task_code, status, message, now, now, 0_i64, now),
    )?;
    Ok(())
}

#[derive(Debug, Clone, Serialize)]
pub struct PostListItem {
    pub id: i64,
    pub title: String,
    pub slug: String,
    pub status: String,
    pub published_at: i64,
}

#[derive(Debug, Clone, Serialize)]
pub struct TaskListItem {
    pub id: i64,
    pub code: String,
    pub name: String,
    pub schedule: String,
    pub enabled: i64,
    pub kind: i64,
    pub last_status: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct TaskRunListItem {
    pub id: i64,
    pub task_code: String,
    pub status: String,
    pub message: String,
    pub created_at: i64,
}

#[derive(Debug, Clone, Serialize)]
pub struct TaskDetail {
    pub id: i64,
    pub code: String,
    pub name: String,
    pub description: String,
    pub schedule: String,
    pub enabled: i64,
    pub kind: i64,
    pub last_status: String,
}

#[derive(Debug, Clone)]
pub struct TaskDefinition {
    pub code: &'static str,
    pub name: &'static str,
    pub description: &'static str,
    pub schedule: &'static str,
    pub enabled: i64,
    pub kind: i64,
}

#[derive(Debug, Clone)]
pub struct TaskRecord {
    pub code: String,
    pub schedule: String,
}

#[derive(Debug, Clone)]
pub struct TaskMutation {
    pub code: String,
    pub name: String,
    pub description: String,
    pub schedule: String,
    pub enabled: i64,
    pub kind: i64,
}

pub fn list_posts(conn: &Connection, limit: usize) -> Result<Vec<PostListItem>> {
    let mut stmt = conn.prepare(
        "SELECT id, title, slug, status, published_at
         FROM t_posts
         WHERE deleted_at IS NULL
         ORDER BY id DESC
         LIMIT ?1",
    )?;
    let rows = stmt.query_map((limit as i64,), |row| {
        Ok(PostListItem {
            id: row.get(0)?,
            title: row.get(1)?,
            slug: row.get(2)?,
            status: row.get(3)?,
            published_at: row.get(4)?,
        })
    })?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row?);
    }
    Ok(items)
}

pub fn list_tasks(conn: &Connection, limit: usize) -> Result<Vec<TaskListItem>> {
    let mut stmt = conn.prepare(
        "SELECT id, code, name, schedule, enabled, kind, COALESCE(last_status, '')
         FROM t_tasks
         WHERE deleted_at IS NULL
         ORDER BY id DESC
         LIMIT ?1",
    )?;
    let rows = stmt.query_map((limit as i64,), |row| {
        Ok(TaskListItem {
            id: row.get(0)?,
            code: row.get(1)?,
            name: row.get(2)?,
            schedule: row.get(3)?,
            enabled: row.get(4)?,
            kind: row.get(5)?,
            last_status: row.get(6)?,
        })
    })?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row?);
    }
    Ok(items)
}

pub fn list_task_runs(
    conn: &Connection,
    task_code: &str,
    limit: usize,
) -> Result<Vec<TaskRunListItem>> {
    let mut stmt = conn.prepare(
        "SELECT id, task_code, status, message, created_at
         FROM t_task_runs
         WHERE task_code = ?1
         ORDER BY id DESC
         LIMIT ?2",
    )?;
    let rows = stmt.query_map((task_code, limit as i64), |row| {
        Ok(TaskRunListItem {
            id: row.get(0)?,
            task_code: row.get(1)?,
            status: row.get(2)?,
            message: row.get(3)?,
            created_at: row.get(4)?,
        })
    })?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row?);
    }
    Ok(items)
}

pub fn list_enabled_task_records(conn: &Connection) -> Result<Vec<TaskRecord>> {
    let mut stmt = conn.prepare(
        "SELECT code, schedule
         FROM t_tasks
         WHERE deleted_at IS NULL AND enabled = 1
         ORDER BY id ASC",
    )?;
    let rows = stmt.query_map([], |row| {
        Ok(TaskRecord {
            code: row.get(0)?,
            schedule: row.get(1)?,
        })
    })?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row?);
    }
    Ok(items)
}

pub fn get_task_by_code(conn: &Connection, task_code: &str) -> Result<Option<TaskDetail>> {
    conn.query_row(
        "SELECT id, code, name, COALESCE(description, ''), schedule, enabled, kind, COALESCE(last_status, '')
         FROM t_tasks
         WHERE code = ?1 AND deleted_at IS NULL
         LIMIT 1",
        [task_code],
        |row| {
            Ok(TaskDetail {
                id: row.get(0)?,
                code: row.get(1)?,
                name: row.get(2)?,
                description: row.get(3)?,
                schedule: row.get(4)?,
                enabled: row.get(5)?,
                kind: row.get(6)?,
                last_status: row.get(7)?,
            })
        },
    )
    .optional()
    .map_err(Into::into)
}

pub fn get_task_by_id(conn: &Connection, task_id: i64) -> Result<Option<TaskDetail>> {
    conn.query_row(
        "SELECT id, code, name, COALESCE(description, ''), schedule, enabled, kind, COALESCE(last_status, '')
         FROM t_tasks
         WHERE id = ?1 AND deleted_at IS NULL
         LIMIT 1",
        [task_id],
        |row| {
            Ok(TaskDetail {
                id: row.get(0)?,
                code: row.get(1)?,
                name: row.get(2)?,
                description: row.get(3)?,
                schedule: row.get(4)?,
                enabled: row.get(5)?,
                kind: row.get(6)?,
                last_status: row.get(7)?,
            })
        },
    )
    .optional()
    .map_err(Into::into)
}

pub fn create_task(conn: &Connection, task: &TaskMutation) -> Result<i64> {
    let now = unix_now();
    conn.execute(
        "INSERT INTO t_tasks(code, name, description, schedule, enabled, kind, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
        (
            task.code.as_str(),
            task.name.as_str(),
            task.description.as_str(),
            task.schedule.as_str(),
            task.enabled,
            task.kind,
            now,
            now,
        ),
    )?;
    Ok(conn.last_insert_rowid())
}

pub fn update_task(conn: &Connection, task_id: i64, task: &TaskMutation) -> Result<()> {
    let now = unix_now();
    conn.execute(
        "UPDATE t_tasks
         SET name = ?2, description = ?3, schedule = ?4, enabled = ?5, kind = ?6, updated_at = ?7
         WHERE id = ?1 AND deleted_at IS NULL",
        (
            task_id,
            task.name.as_str(),
            task.description.as_str(),
            task.schedule.as_str(),
            task.enabled,
            task.kind,
            now,
        ),
    )?;
    Ok(())
}

pub fn soft_delete_task(conn: &Connection, task_id: i64) -> Result<()> {
    let now = unix_now();
    conn.execute(
        "UPDATE t_tasks
         SET deleted_at = ?2, updated_at = ?2
         WHERE id = ?1 AND deleted_at IS NULL",
        (task_id, now),
    )?;
    Ok(())
}

pub fn ensure_builtin_task(conn: &Connection, task: &TaskDefinition) -> Result<()> {
    let existing: i64 = conn.query_row(
        "SELECT COUNT(*) FROM t_tasks WHERE code = ?1 AND deleted_at IS NULL",
        [task.code],
        |row| row.get(0),
    )?;
    if existing > 0 {
        return Ok(());
    }

    let now = unix_now();
    conn.execute(
        "INSERT INTO t_tasks(code, name, description, schedule, enabled, kind, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
        (
            task.code,
            task.name,
            task.description,
            task.schedule,
            task.enabled,
            task.kind,
            now,
            now,
        ),
    )?;
    Ok(())
}

pub fn update_task_status(
    conn: &Connection,
    task_code: &str,
    status: &str,
    last_run_at: i64,
) -> Result<()> {
    conn.execute(
        "UPDATE t_tasks
         SET last_status = ?2, last_run_at = ?3, updated_at = ?3
         WHERE code = ?1 AND deleted_at IS NULL",
        (task_code, status, last_run_at),
    )?;
    Ok(())
}

#[cfg(test)]
pub fn count_tasks_by_code(conn: &Connection, task_code: &str) -> Result<i64> {
    conn.query_row(
        "SELECT COUNT(*) FROM t_tasks WHERE code = ?1 AND deleted_at IS NULL",
        [task_code],
        |row| row.get(0),
    )
    .map_err(Into::into)
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

    #[test]
    fn initializes_real_go_schema() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");

        for table in [
            "t_posts",
            "t_categories",
            "t_settings",
            "t_task_runs",
            "t_themes",
        ] {
            let exists: i64 = conn
                .query_row(
                    "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?1",
                    [table],
                    |row| row.get(0),
                )
                .expect("query sqlite_master");
            assert_eq!(exists, 1, "expected table {table} to exist");
        }
    }

    #[test]
    fn records_task_run_using_real_schema_columns() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");

        record_task_run(&conn, "heartbeat", "ok", "scheduler tick").expect("insert task run");

        let row: (String, String, String, i64) = conn
            .query_row(
                "SELECT task_code, status, message, duration FROM t_task_runs ORDER BY id DESC LIMIT 1",
                [],
                |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
            )
            .expect("read task run");

        assert_eq!(row.0, "heartbeat");
        assert_eq!(row.1, "ok");
        assert_eq!(row.2, "scheduler tick");
        assert_eq!(row.3, 0);
    }

    #[test]
    fn ensure_builtin_task_inserts_only_once() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");

        let task = TaskDefinition {
            code: "check_app_update",
            name: "检查应用更新",
            description: "每天检查 swaves 是否有新的稳定版本可升级",
            schedule: "@daily",
            enabled: 1,
            kind: 0,
        };

        ensure_builtin_task(&conn, &task).expect("insert builtin task first time");
        ensure_builtin_task(&conn, &task).expect("insert builtin task second time");

        let count = count_tasks_by_code(&conn, task.code).expect("count builtin tasks");
        assert_eq!(count, 1);
    }

    #[test]
    fn update_task_status_writes_last_status_and_time() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");
        let task = TaskDefinition {
            code: "clear_notifications",
            name: "清理过期通知",
            description: "按消息通知设置中的保留天数清理过期通知",
            schedule: "@daily",
            enabled: 1,
            kind: 0,
        };
        ensure_builtin_task(&conn, &task).expect("insert builtin task");

        update_task_status(&conn, task.code, "success", 123).expect("update task status");

        let row: (String, i64) = conn
            .query_row(
                "SELECT last_status, last_run_at FROM t_tasks WHERE code = ?1",
                [task.code],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .expect("read task status");
        assert_eq!(row.0, "success");
        assert_eq!(row.1, 123);
    }

    #[test]
    fn get_task_by_code_returns_existing_task() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");
        let task = TaskDefinition {
            code: "clear_notifications",
            name: "清理过期通知",
            description: "按保留天数清理过期通知",
            schedule: "@daily",
            enabled: 1,
            kind: 0,
        };
        ensure_builtin_task(&conn, &task).expect("insert builtin task");

        let fetched = get_task_by_code(&conn, task.code)
            .expect("query task by code")
            .expect("task should exist");
        assert_eq!(fetched.code, task.code);
        assert_eq!(fetched.name, task.name);
        assert_eq!(fetched.schedule, task.schedule);
        assert_eq!(fetched.enabled, task.enabled);
    }

    #[test]
    fn create_update_and_soft_delete_task_follow_expected_semantics() {
        let conn = Connection::open_in_memory().expect("open in-memory sqlite");
        conn.execute_batch(INITIAL_SQL)
            .expect("initialize schema from Go InitialSQL");

        let task_id = create_task(
            &conn,
            &TaskMutation {
                code: "user_task".to_string(),
                name: "User Task".to_string(),
                description: "created in rust test".to_string(),
                schedule: "@hourly".to_string(),
                enabled: 1,
                kind: 1,
            },
        )
        .expect("create task");

        let created = get_task_by_id(&conn, task_id)
            .expect("query task by id")
            .expect("task should exist");
        assert_eq!(created.code, "user_task");
        assert_eq!(created.kind, 1);

        update_task(
            &conn,
            task_id,
            &TaskMutation {
                code: "ignored".to_string(),
                name: "Renamed Task".to_string(),
                description: "updated".to_string(),
                schedule: "@daily".to_string(),
                enabled: 0,
                kind: 1,
            },
        )
        .expect("update task");

        let updated = get_task_by_id(&conn, task_id)
            .expect("query updated task by id")
            .expect("task should still exist");
        assert_eq!(updated.code, "user_task");
        assert_eq!(updated.name, "Renamed Task");
        assert_eq!(updated.schedule, "@daily");
        assert_eq!(updated.enabled, 0);

        soft_delete_task(&conn, task_id).expect("soft delete task");
        assert!(get_task_by_id(&conn, task_id)
            .expect("query deleted task by id")
            .is_none());
    }
}
