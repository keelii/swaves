use rusqlite::Connection;

pub fn open(dsn: &str) -> rusqlite::Result<()> {
    let conn = Connection::open(dsn)?;
    conn.execute_batch("PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;")?;
    Ok(())
}
