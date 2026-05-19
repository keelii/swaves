use anyhow::Result;
use rusqlite::Connection;

// POC note: this keeps the same intent as Go InitialSQL and should be replaced
// with a generated/verified full schema copy in the next iteration.
const INITIAL_SQL_POC: &str = r#"
CREATE TABLE IF NOT EXISTS t_tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS t_task_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_code TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
"#;

pub fn open_and_init(sqlite_file: &str) -> Result<Connection> {
    let conn = Connection::open(sqlite_file)?;
    conn.execute_batch(INITIAL_SQL_POC)?;
    Ok(conn)
}

pub fn record_task_run(conn: &Connection, task_code: &str, status: &str, detail: &str) -> Result<()> {
    conn.execute(
        "INSERT INTO t_task_runs(task_code, status, detail) VALUES (?1, ?2, ?3)",
        (task_code, status, detail),
    )?;
    Ok(())
}
