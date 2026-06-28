//! Session and message persistence.

use std::path::Path;

use candela_core::harness::{HarnessError, Message, MessageRole, Session};
use rusqlite::{Connection, params};
use tracing::info;

/// SQLite database for session and message storage.
pub struct Database {
    conn: Connection,
}

impl Database {
    /// Open or create the database at the given path.
    pub fn open(path: &Path) -> Result<Self, HarnessError> {
        let conn = Connection::open(path).map_err(|e| HarnessError::Storage(e.to_string()))?;
        let db = Self { conn };
        db.init_schema()?;
        info!(path = %path.display(), "database opened");
        Ok(db)
    }

    /// Open an in-memory database (for testing).
    pub fn open_in_memory() -> Result<Self, HarnessError> {
        let conn =
            Connection::open_in_memory().map_err(|e| HarnessError::Storage(e.to_string()))?;
        let db = Self { conn };
        db.init_schema()?;
        Ok(db)
    }

    fn init_schema(&self) -> Result<(), HarnessError> {
        self.conn
            .execute_batch(
                "
            CREATE TABLE IF NOT EXISTS sessions (
                id TEXT PRIMARY KEY,
                title TEXT NOT NULL DEFAULT 'New Chat',
                model TEXT NOT NULL DEFAULT '',
                message_count INTEGER NOT NULL DEFAULT 0,
                total_tokens INTEGER NOT NULL DEFAULT 0,
                total_cost_usd REAL NOT NULL DEFAULT 0.0,
                device_id TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                deleted_at TEXT
            );

            CREATE TABLE IF NOT EXISTS messages (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
                role TEXT NOT NULL,
                content TEXT NOT NULL,
                model TEXT,
                token_count INTEGER,
                cost_usd REAL,
                created_at TEXT NOT NULL
            );

            CREATE INDEX IF NOT EXISTS idx_messages_session
                ON messages(session_id, created_at);
        ",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        Ok(())
    }

    // -- Sessions --

    /// Insert a new session.
    pub fn create_session(&self, session: &Session) -> Result<(), HarnessError> {
        self.conn
            .execute(
                "INSERT INTO sessions (id, title, model, message_count, total_tokens, total_cost_usd, device_id, created_at, updated_at, deleted_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)",
                params![
                    session.id,
                    session.title,
                    session.model,
                    session.message_count,
                    session.total_tokens,
                    session.total_cost_usd,
                    session.device_id,
                    session.created_at.to_rfc3339(),
                    session.updated_at.to_rfc3339(),
                    session.deleted_at.map(|dt| dt.to_rfc3339()),
                ],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        Ok(())
    }

    /// List all non-deleted sessions, most recent first.
    pub fn list_sessions(&self, limit: i64, offset: i64) -> Result<Vec<Session>, HarnessError> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, title, model, message_count, total_tokens, total_cost_usd, device_id, created_at, updated_at, deleted_at
             FROM sessions
             WHERE deleted_at IS NULL
             ORDER BY updated_at DESC
             LIMIT ?1 OFFSET ?2",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        let sessions = stmt
            .query_map(params![limit, offset], |row| {
                Ok(Session {
                    id: row.get(0)?,
                    title: row.get(1)?,
                    model: row.get(2)?,
                    message_count: row.get(3)?,
                    total_tokens: row.get(4)?,
                    total_cost_usd: row.get(5)?,
                    device_id: row.get(6)?,
                    created_at: row.get::<_, String>(7)?.parse().unwrap_or_default(),
                    updated_at: row.get::<_, String>(8)?.parse().unwrap_or_default(),
                    deleted_at: row
                        .get::<_, Option<String>>(9)?
                        .and_then(|s| s.parse().ok()),
                })
            })
            .map_err(|e| HarnessError::Storage(e.to_string()))?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(sessions)
    }

    /// Soft-delete a session.
    pub fn delete_session(&self, id: &str) -> Result<(), HarnessError> {
        let now = chrono::Utc::now().to_rfc3339();
        let rows = self
            .conn
            .execute(
                "UPDATE sessions SET deleted_at = ?1 WHERE id = ?2 AND deleted_at IS NULL",
                params![now, id],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        if rows == 0 {
            return Err(HarnessError::SessionNotFound(id.to_string()));
        }
        Ok(())
    }

    // -- Messages --

    /// Insert a message and increment session counters.
    pub fn insert_message(&self, msg: &Message) -> Result<i64, HarnessError> {
        self.conn
            .execute(
                "INSERT INTO messages (session_id, role, content, model, token_count, cost_usd, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
                params![
                    msg.session_id,
                    serde_json::to_string(&msg.role)
                        .unwrap_or_default()
                        .trim_matches('"'),
                    msg.content,
                    msg.model,
                    msg.token_count,
                    msg.cost_usd,
                    msg.created_at.to_rfc3339(),
                ],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        let id = self.conn.last_insert_rowid();

        // Update session counters
        self.conn
            .execute(
                "UPDATE sessions SET message_count = message_count + 1, updated_at = ?1, total_tokens = total_tokens + ?2, total_cost_usd = total_cost_usd + ?3 WHERE id = ?4",
                params![
                    chrono::Utc::now().to_rfc3339(),
                    msg.token_count.unwrap_or(0),
                    msg.cost_usd.unwrap_or(0.0),
                    msg.session_id,
                ],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(id)
    }

    /// Get messages for a session.
    pub fn get_messages(&self, session_id: &str, limit: i64) -> Result<Vec<Message>, HarnessError> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, session_id, role, content, model, token_count, cost_usd, created_at
             FROM messages
             WHERE session_id = ?1
             ORDER BY created_at ASC
             LIMIT ?2",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        let messages = stmt
            .query_map(params![session_id, limit], |row| {
                let role_str: String = row.get(2)?;
                let role = match role_str.as_str() {
                    "user" => MessageRole::User,
                    "assistant" => MessageRole::Assistant,
                    "system" => MessageRole::System,
                    "tool" => MessageRole::Tool,
                    _ => MessageRole::User,
                };
                Ok(Message {
                    id: row.get(0)?,
                    session_id: row.get(1)?,
                    role,
                    content: row.get(3)?,
                    model: row.get(4)?,
                    token_count: row.get(5)?,
                    cost_usd: row.get(6)?,
                    created_at: row.get::<_, String>(7)?.parse().unwrap_or_default(),
                })
            })
            .map_err(|e| HarnessError::Storage(e.to_string()))?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(messages)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use candela_core::harness::Session;

    #[test]
    fn test_create_and_list_sessions() {
        let db = Database::open_in_memory().unwrap();
        let session = Session::new("gemini-2.0-flash", "device-1");
        db.create_session(&session).unwrap();

        let sessions = db.list_sessions(50, 0).unwrap();
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].id, session.id);
        assert_eq!(sessions[0].title, "New Chat");
    }

    #[test]
    fn test_soft_delete_session() {
        let db = Database::open_in_memory().unwrap();
        let session = Session::new("test-model", "device-1");
        db.create_session(&session).unwrap();

        db.delete_session(&session.id).unwrap();

        let sessions = db.list_sessions(50, 0).unwrap();
        assert_eq!(sessions.len(), 0); // soft-deleted, not visible
    }

    #[test]
    fn test_insert_and_get_messages() {
        let db = Database::open_in_memory().unwrap();
        let session = Session::new("test-model", "device-1");
        db.create_session(&session).unwrap();

        let msg = Message {
            id: 0,
            session_id: session.id.clone(),
            role: MessageRole::User,
            content: "Hello!".to_string(),
            model: None,
            token_count: Some(5),
            cost_usd: Some(0.001),
            created_at: chrono::Utc::now(),
        };
        db.insert_message(&msg).unwrap();

        let messages = db.get_messages(&session.id, 50).unwrap();
        assert_eq!(messages.len(), 1);
        assert_eq!(messages[0].content, "Hello!");
    }
}
