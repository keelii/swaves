use anyhow::Result;
use rusqlite::Connection;
use serde::Serialize;

const INITIAL_SQL: &str = include_str!(concat!(env!("OUT_DIR"), "/initial_sql.sql"));

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
    pub code: String,
    pub name: String,
    pub enabled: i64,
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
        "SELECT code, name, enabled, COALESCE(last_status, '')
         FROM t_tasks
         WHERE deleted_at IS NULL
         ORDER BY id DESC
         LIMIT ?1",
    )?;
    let rows = stmt.query_map((limit as i64,), |row| {
        Ok(TaskListItem {
            code: row.get(0)?,
            name: row.get(1)?,
            enabled: row.get(2)?,
            last_status: row.get(3)?,
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
}
