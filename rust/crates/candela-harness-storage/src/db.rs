//! Session and message persistence.

use std::path::Path;

use candela_core::harness::{HarnessError, Message, MessageRole, Session};
use chrono::{DateTime, Utc};
use rusqlite::{Connection, params};
use tracing::info;

/// Convert a MessageRole to a lowercase string for DB storage.
fn role_to_str(role: &MessageRole) -> &'static str {
    match role {
        MessageRole::User => "user",
        MessageRole::Assistant => "assistant",
        MessageRole::System => "system",
        MessageRole::Tool => "tool",
        MessageRole::Unspecified => "unspecified",
    }
}

/// Parse a role string from the DB back to MessageRole.
fn role_from_str(s: &str) -> MessageRole {
    match s {
        "user" => MessageRole::User,
        "assistant" => MessageRole::Assistant,
        "system" => MessageRole::System,
        "tool" => MessageRole::Tool,
        _ => MessageRole::Unspecified,
    }
}

/// SQLite database for session and message storage.
pub struct Database {
    conn: Connection,
}

impl Database {
    /// Open or create the database at the given path.
    pub fn open(path: &Path) -> Result<Self, HarnessError> {
        let conn = Connection::open(path).map_err(|e| HarnessError::Storage(e.to_string()))?;
        conn.execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        let db = Self { conn };
        db.init_schema()?;
        info!(path = %path.display(), "database opened");
        Ok(db)
    }

    /// Open an in-memory database (for testing).
    pub fn open_in_memory() -> Result<Self, HarnessError> {
        let conn =
            Connection::open_in_memory().map_err(|e| HarnessError::Storage(e.to_string()))?;
        conn.execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        let db = Self { conn };
        db.init_schema()?;
        Ok(db)
    }

    fn init_schema(&self) -> Result<(), HarnessError> {
        let version: i32 = self
            .conn
            .pragma_query_value(None, "user_version", |row| row.get(0))
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        if version < 1 {
            self.conn
                .execute_batch(
                    "
                -- Drop legacy tables if they exist (pre-v1 schema).
                DROP TABLE IF EXISTS messages;
                DROP TABLE IF EXISTS sessions;

                CREATE TABLE sessions (
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

                CREATE TABLE messages (
                    id TEXT PRIMARY KEY,
                    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
                    role TEXT NOT NULL,
                    content TEXT NOT NULL,
                    model TEXT,
                    token_count INTEGER,
                    cost_usd REAL,
                    created_at TEXT NOT NULL,
                    sequence INTEGER NOT NULL DEFAULT 0
                );

                CREATE INDEX idx_messages_session
                    ON messages(session_id, sequence);

                PRAGMA user_version = 1;
            ",
                )
                .map_err(|e| HarnessError::Storage(e.to_string()))?;
        }

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
                    created_at: row.get::<_, DateTime<Utc>>(7)?,
                    updated_at: row.get::<_, DateTime<Utc>>(8)?,
                    deleted_at: row.get::<_, Option<DateTime<Utc>>>(9)?,
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
    ///
    /// Uses a transaction to ensure atomicity between the message insert and
    /// session counter update. Verifies the target session exists and is not
    /// soft-deleted.
    pub fn insert_message(&mut self, msg: &Message) -> Result<String, HarnessError> {
        let tx = self
            .conn
            .transaction()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        // Auto-assign sequence: next value for this session.
        let next_seq: i64 = tx
            .query_row(
                "SELECT COALESCE(MAX(sequence), 0) + 1 FROM messages WHERE session_id = ?1",
                params![msg.session_id],
                |row| row.get(0),
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        tx.execute(
            "INSERT INTO messages (id, session_id, role, content, model, token_count, cost_usd, created_at, sequence)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)",
            params![
                msg.id,
                msg.session_id,
                role_to_str(&msg.role),
                msg.content,
                msg.model,
                msg.token_count,
                msg.cost_usd,
                msg.created_at.to_rfc3339(),
                next_seq,
            ],
        )
        .map_err(|e| HarnessError::Storage(e.to_string()))?;

        // Update session counters — only for non-deleted sessions.
        let rows_updated = tx
            .execute(
                "UPDATE sessions SET message_count = message_count + 1, updated_at = ?1, total_tokens = total_tokens + ?2, total_cost_usd = total_cost_usd + ?3 WHERE id = ?4 AND deleted_at IS NULL",
                params![
                    chrono::Utc::now().to_rfc3339(),
                    msg.token_count.unwrap_or(0),
                    msg.cost_usd.unwrap_or(0.0),
                    msg.session_id,
                ],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        if rows_updated != 1 {
            return Err(HarnessError::SessionNotFound(msg.session_id.clone()));
        }

        tx.commit()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(msg.id.clone())
    }

    /// Get messages for a session.
    pub fn get_messages(&self, session_id: &str, limit: i64) -> Result<Vec<Message>, HarnessError> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, session_id, role, content, model, token_count, cost_usd, created_at, sequence
             FROM messages
             WHERE session_id = ?1
             ORDER BY sequence ASC, created_at ASC
             LIMIT ?2",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        let messages = stmt
            .query_map(params![session_id, limit], |row| {
                let role_str: String = row.get(2)?;
                let role = role_from_str(&role_str);
                Ok(Message {
                    id: row.get(0)?,
                    session_id: row.get(1)?,
                    role,
                    content: row.get(3)?,
                    model: row.get(4)?,
                    token_count: row.get(5)?,
                    cost_usd: row.get(6)?,
                    created_at: row.get::<_, DateTime<Utc>>(7)?,
                    sequence: row.get(8)?,
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
    use candela_core::harness::{new_message, new_session};

    #[test]
    fn test_create_and_list_sessions() {
        let db = Database::open_in_memory().unwrap();
        let session = new_session("gemini-2.0-flash", "device-1");
        db.create_session(&session).unwrap();

        let sessions = db.list_sessions(50, 0).unwrap();
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].id, session.id);
        assert_eq!(sessions[0].title, "New Chat");
    }

    #[test]
    fn test_soft_delete_session() {
        let db = Database::open_in_memory().unwrap();
        let session = new_session("test-model", "device-1");
        db.create_session(&session).unwrap();

        db.delete_session(&session.id).unwrap();

        let sessions = db.list_sessions(50, 0).unwrap();
        assert_eq!(sessions.len(), 0); // soft-deleted, not visible
    }

    #[test]
    fn test_insert_and_get_messages() {
        let mut db = Database::open_in_memory().unwrap();
        let session = new_session("test-model", "device-1");
        db.create_session(&session).unwrap();

        let msg = new_message(&session.id, MessageRole::User, "Hello!");
        db.insert_message(&msg).unwrap();

        let messages = db.get_messages(&session.id, 50).unwrap();
        assert_eq!(messages.len(), 1);
        assert_eq!(messages[0].content, "Hello!");
    }

    #[test]
    fn test_message_sequence_auto_assign() {
        let mut db = Database::open_in_memory().unwrap();
        let session = new_session("test-model", "device-1");
        db.create_session(&session).unwrap();

        let m1 = new_message(&session.id, MessageRole::User, "first");
        let m2 = new_message(&session.id, MessageRole::Assistant, "second");
        let m3 = new_message(&session.id, MessageRole::User, "third");
        db.insert_message(&m1).unwrap();
        db.insert_message(&m2).unwrap();
        db.insert_message(&m3).unwrap();

        let messages = db.get_messages(&session.id, 50).unwrap();
        assert_eq!(messages.len(), 3);
        assert_eq!(messages[0].sequence, 1);
        assert_eq!(messages[1].sequence, 2);
        assert_eq!(messages[2].sequence, 3);
    }

    #[test]
    fn test_message_role_round_trip() {
        let mut db = Database::open_in_memory().unwrap();
        let session = new_session("test-model", "device-1");
        db.create_session(&session).unwrap();

        let roles = [
            MessageRole::User,
            MessageRole::Assistant,
            MessageRole::System,
            MessageRole::Tool,
            MessageRole::Unspecified,
        ];

        for role in &roles {
            let msg = new_message(&session.id, *role, "content");
            db.insert_message(&msg).unwrap();
        }

        let messages = db.get_messages(&session.id, 50).unwrap();
        assert_eq!(messages.len(), roles.len());
        for (msg, expected_role) in messages.iter().zip(roles.iter()) {
            assert_eq!(
                msg.role, *expected_role,
                "role mismatch for {:?}",
                expected_role
            );
        }
    }

    #[test]
    fn test_insert_message_returns_uuid() {
        let mut db = Database::open_in_memory().unwrap();
        let session = new_session("test-model", "device-1");
        db.create_session(&session).unwrap();

        let msg = new_message(&session.id, MessageRole::User, "hello");
        let returned_id = db.insert_message(&msg).unwrap();

        assert!(!returned_id.is_empty());
        assert_eq!(returned_id, msg.id);
    }

    #[test]
    fn test_session_not_found_on_insert() {
        let mut db = Database::open_in_memory().unwrap();

        let msg = new_message("non-existent-session", MessageRole::User, "hello");
        let result = db.insert_message(&msg);

        assert!(result.is_err());
        let err = result.unwrap_err();
        // The FK constraint on session_id fires before the manual row-count check,
        // so we get a Storage error containing "FOREIGN KEY".
        assert!(
            matches!(err, HarnessError::Storage(ref s) if s.contains("FOREIGN KEY"))
                || matches!(err, HarnessError::SessionNotFound(_)),
            "expected FK or SessionNotFound error, got: {err:?}"
        );
    }

    #[test]
    fn test_multiple_sessions_independent_sequence() {
        let mut db = Database::open_in_memory().unwrap();

        let s1 = new_session("model-a", "device-1");
        let s2 = new_session("model-b", "device-1");
        db.create_session(&s1).unwrap();
        db.create_session(&s2).unwrap();

        // Insert 2 messages in session 1, then 1 in session 2.
        db.insert_message(&new_message(&s1.id, MessageRole::User, "s1-m1"))
            .unwrap();
        db.insert_message(&new_message(&s1.id, MessageRole::Assistant, "s1-m2"))
            .unwrap();
        db.insert_message(&new_message(&s2.id, MessageRole::User, "s2-m1"))
            .unwrap();

        let msgs_s1 = db.get_messages(&s1.id, 50).unwrap();
        let msgs_s2 = db.get_messages(&s2.id, 50).unwrap();

        assert_eq!(msgs_s1.len(), 2);
        assert_eq!(msgs_s1[0].sequence, 1);
        assert_eq!(msgs_s1[1].sequence, 2);

        assert_eq!(msgs_s2.len(), 1);
        assert_eq!(
            msgs_s2[0].sequence, 1,
            "session 2 sequence should start at 1 independently"
        );
    }
}
