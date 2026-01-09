use rusqlite::{Connection, Result, params};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::sync::Mutex;

static DB: once_cell::sync::OnceCell<Mutex<Connection>> = once_cell::sync::OnceCell::new();

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LocalConversation {
    pub conversation_id: String,
    pub name: String,
    pub first_message: String,
    pub message_count: i32,
    pub last_used_time: String,
    pub is_generating: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LocalMessage {
    pub id: i64,
    pub conversation_id: String,
    pub role: String,
    pub content: String,
    pub timestamp: String,
}

pub fn init_db(exe_dir: &Path) -> Result<()> {
    let db_path = exe_dir.join("data.db");
    let conn = Connection::open(&db_path)?;

    conn.execute(
        "CREATE TABLE IF NOT EXISTS conversations (
            id TEXT PRIMARY KEY,
            name TEXT DEFAULT '',
            first_message TEXT DEFAULT '',
            message_count INTEGER DEFAULT 0,
            last_used_time DATETIME DEFAULT CURRENT_TIMESTAMP,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )",
        [],
    )?;

    conn.execute(
        "CREATE TABLE IF NOT EXISTS messages (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            conversation_id TEXT NOT NULL,
            role TEXT NOT NULL,
            content TEXT NOT NULL,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
        )",
        [],
    )?;

    conn.execute(
        "CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id)",
        [],
    )?;

    tracing::info!("Database initialized: {:?}", db_path);
    DB.set(Mutex::new(conn)).map_err(|_| rusqlite::Error::InvalidQuery)?;
    Ok(())
}

fn with_db<F, T>(f: F) -> Result<T>
where
    F: FnOnce(&Connection) -> Result<T>,
{
    let db = DB.get().ok_or(rusqlite::Error::InvalidQuery)?;
    let conn = db.lock().map_err(|_| rusqlite::Error::InvalidQuery)?;
    f(&conn)
}

pub fn get_conversations() -> Result<Vec<LocalConversation>> {
    with_db(|conn| {
        let mut stmt = conn.prepare(
            "SELECT id, name, first_message, message_count, last_used_time
             FROM conversations ORDER BY last_used_time DESC"
        )?;

        let rows = stmt.query_map([], |row| {
            Ok(LocalConversation {
                conversation_id: row.get(0)?,
                name: row.get(1)?,
                first_message: row.get(2)?,
                message_count: row.get(3)?,
                last_used_time: row.get(4)?,
                is_generating: false,
            })
        })?;

        rows.collect()
    })
}

pub fn create_conversation(id: &str, first_message: &str) -> Result<()> {
    with_db(|conn| {
        conn.execute(
            "INSERT INTO conversations (id, first_message, message_count, last_used_time)
             VALUES (?1, ?2, 0, CURRENT_TIMESTAMP)",
            params![id, first_message],
        )?;
        Ok(())
    })
}

pub fn update_conversation_name(id: &str, name: &str) -> Result<()> {
    with_db(|conn| {
        conn.execute(
            "UPDATE conversations SET name = ?1 WHERE id = ?2",
            params![name, id],
        )?;
        Ok(())
    })
}

pub fn delete_conversation(id: &str) -> Result<()> {
    with_db(|conn| {
        conn.execute("DELETE FROM messages WHERE conversation_id = ?1", params![id])?;
        conn.execute("DELETE FROM conversations WHERE id = ?1", params![id])?;
        Ok(())
    })
}

pub fn get_messages(conversation_id: &str) -> Result<Vec<LocalMessage>> {
    with_db(|conn| {
        let mut stmt = conn.prepare(
            "SELECT id, conversation_id, role, content, timestamp
             FROM messages WHERE conversation_id = ?1 ORDER BY timestamp ASC"
        )?;

        let rows = stmt.query_map(params![conversation_id], |row| {
            Ok(LocalMessage {
                id: row.get(0)?,
                conversation_id: row.get(1)?,
                role: row.get(2)?,
                content: row.get(3)?,
                timestamp: row.get(4)?,
            })
        })?;

        rows.collect()
    })
}

pub fn add_message(conversation_id: &str, role: &str, content: &str) -> Result<()> {
    with_db(|conn| {
        conn.execute(
            "INSERT INTO messages (conversation_id, role, content, timestamp)
             VALUES (?1, ?2, ?3, CURRENT_TIMESTAMP)",
            params![conversation_id, role, content],
        )?;

        conn.execute(
            "UPDATE conversations SET message_count = message_count + 1, last_used_time = CURRENT_TIMESTAMP WHERE id = ?1",
            params![conversation_id],
        )?;

        // Update first_message if this is the first user message
        if role == "user" {
            let count: i32 = conn.query_row(
                "SELECT COUNT(*) FROM messages WHERE conversation_id = ?1 AND role = 'user'",
                params![conversation_id],
                |row| row.get(0),
            )?;

            if count == 1 {
                conn.execute(
                    "UPDATE conversations SET first_message = ?1 WHERE id = ?2",
                    params![content, conversation_id],
                )?;
            }
        }

        Ok(())
    })
}

pub fn conversation_exists(id: &str) -> bool {
    with_db(|conn| {
        let count: i32 = conn.query_row(
            "SELECT COUNT(*) FROM conversations WHERE id = ?1",
            params![id],
            |row| row.get(0),
        ).unwrap_or(0);
        Ok(count > 0)
    }).unwrap_or(false)
}
