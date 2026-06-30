//! FTS5 full-text search index for messages.

use candela_core::harness::{HarnessError, MessageRole, SearchResult};
use chrono::{DateTime, Utc};
use rusqlite::{Connection, params};
use tracing::info;

/// FTS5 search index for cross-session message search.
pub struct SearchIndex {
    conn: Connection,
}

impl SearchIndex {
    /// Open or create the search index at the given path.
    pub fn open(path: &std::path::Path) -> Result<Self, HarnessError> {
        let conn = Connection::open(path).map_err(|e| HarnessError::Storage(e.to_string()))?;
        let idx = Self { conn };
        idx.init_schema()?;
        info!(path = %path.display(), "search index opened");
        Ok(idx)
    }

    /// Open an in-memory search index (for testing).
    pub fn open_in_memory() -> Result<Self, HarnessError> {
        let conn =
            Connection::open_in_memory().map_err(|e| HarnessError::Storage(e.to_string()))?;
        let idx = Self { conn };
        idx.init_schema()?;
        Ok(idx)
    }

    fn init_schema(&self) -> Result<(), HarnessError> {
        let version: i32 = self
            .conn
            .pragma_query_value(None, "user_version", |row| row.get(0))
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        if version < 1 {
            // Drop and recreate to ensure message_id column exists.
            self.conn
                .execute_batch(
                    "
                DROP TABLE IF EXISTS message_fts;
                DROP TABLE IF EXISTS deleted_sessions;

                CREATE VIRTUAL TABLE message_fts USING fts5(
                    content,
                    session_id UNINDEXED,
                    message_id UNINDEXED,
                    session_title UNINDEXED,
                    role UNINDEXED,
                    created_at UNINDEXED
                );

                CREATE TABLE deleted_sessions (
                    session_id TEXT PRIMARY KEY
                );

                PRAGMA user_version = 1;
            ",
                )
                .map_err(|e| HarnessError::Storage(e.to_string()))?;
        }

        Ok(())
    }

    /// Index a message for full-text search.
    pub fn index_message(
        &self,
        content: &str,
        session_id: &str,
        message_id: &str,
        session_title: &str,
        role: &str,
        created_at: &str,
    ) -> Result<(), HarnessError> {
        self.conn
            .execute(
                "INSERT INTO message_fts (content, session_id, message_id, session_title, role, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                params![content, session_id, message_id, session_title, role, created_at],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        Ok(())
    }

    /// Mark a session as deleted, excluding its messages from future searches.
    pub fn mark_session_deleted(&self, session_id: &str) -> Result<(), HarnessError> {
        self.conn
            .execute(
                "INSERT OR IGNORE INTO deleted_sessions (session_id) VALUES (?1)",
                params![session_id],
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;
        Ok(())
    }

    /// Search messages across all non-deleted sessions.
    pub fn search(&self, query: &str, limit: i64) -> Result<Vec<SearchResult>, HarnessError> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT f.content, f.session_id, f.message_id, f.session_title, f.role, f.created_at, f.rank
             FROM message_fts f
             WHERE f.message_fts MATCH ?1
               AND f.session_id NOT IN (SELECT session_id FROM deleted_sessions)
             ORDER BY f.rank
             LIMIT ?2",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        let results = stmt
            .query_map(params![query, limit], |row| {
                let role_str: String = row.get(4)?;
                let role = match role_str.as_str() {
                    "user" => MessageRole::User,
                    "assistant" => MessageRole::Assistant,
                    "system" => MessageRole::System,
                    "tool" => MessageRole::Tool,
                    _ => MessageRole::Unspecified,
                };
                Ok(SearchResult {
                    message_preview: row.get(0)?,
                    session_id: row.get(1)?,
                    message_id: row.get(2)?,
                    session_title: row.get(3)?,
                    role,
                    created_at: row.get::<_, DateTime<Utc>>(5)?,
                    score: row.get(6)?,
                })
            })
            .map_err(|e| HarnessError::Storage(e.to_string()))?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(results)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_index_and_search() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "How do I refactor the auth module?",
            "session-1",
            "msg-1",
            "Auth Refactor",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();
        idx.index_message(
            "Let me analyze the database schema.",
            "session-2",
            "msg-2",
            "DB Analysis",
            "assistant",
            "2025-01-02T00:00:00Z",
        )
        .unwrap();

        let results = idx.search("refactor auth", 10).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].session_id, "session-1");
        assert_eq!(results[0].message_id, "msg-1");
    }

    #[test]
    fn test_deleted_sessions_excluded_from_search() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "How do I refactor the auth module?",
            "session-1",
            "msg-1",
            "Auth Refactor",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();

        // Before deletion, the message is findable.
        let results = idx.search("refactor auth", 10).unwrap();
        assert_eq!(results.len(), 1);

        // Mark session as deleted.
        idx.mark_session_deleted("session-1").unwrap();

        // After deletion, the message is excluded.
        let results = idx.search("refactor auth", 10).unwrap();
        assert_eq!(results.len(), 0);
    }
}
