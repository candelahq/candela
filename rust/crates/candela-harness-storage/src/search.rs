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
                let mut sr = SearchResult::default();
                sr.message_preview = row.get(0)?;
                sr.session_id = row.get(1)?;
                sr.message_id = row.get(2)?;
                sr.session_title = row.get(3)?;
                sr.role = role;
                sr.created_at = row.get::<_, DateTime<Utc>>(5)?;
                sr.score = row.get(6)?;
                Ok(sr)
            })
            .map_err(|e| HarnessError::Storage(e.to_string()))?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        Ok(results)
    }

    /// Update the session title in all FTS rows for a given session.
    ///
    /// FTS5 doesn't support UPDATE directly, so we use DELETE + re-INSERT
    /// by reading existing rows, deleting them, and re-inserting with the new title.
    pub fn update_session_title(
        &self,
        session_id: &str,
        new_title: &str,
    ) -> Result<(), HarnessError> {
        // Read existing rows for this session
        let mut stmt = self
            .conn
            .prepare(
                "SELECT content, session_id, message_id, session_title, role, created_at, rowid
                 FROM message_fts
                 WHERE session_id = ?1",
            )
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        let rows: Vec<(String, String, String, String, String, String, i64)> = stmt
            .query_map(params![session_id], |row| {
                Ok((
                    row.get(0)?,
                    row.get(1)?,
                    row.get(2)?,
                    row.get(3)?,
                    row.get(4)?,
                    row.get(5)?,
                    row.get(6)?,
                ))
            })
            .map_err(|e| HarnessError::Storage(e.to_string()))?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| HarnessError::Storage(e.to_string()))?;

        // Delete old rows and re-insert with new title
        for (content, sid, mid, _old_title, role, created_at, rowid) in &rows {
            self.conn
                .execute("DELETE FROM message_fts WHERE rowid = ?1", params![rowid])
                .map_err(|e| HarnessError::Storage(e.to_string()))?;

            self.conn
                .execute(
                    "INSERT INTO message_fts (content, session_id, message_id, session_title, role, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                    params![content, sid, mid, new_title, role, created_at],
                )
                .map_err(|e| HarnessError::Storage(e.to_string()))?;
        }

        Ok(())
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

    #[test]
    fn test_search_returns_message_id() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "The quick brown fox jumps over the lazy dog",
            "session-42",
            "msg-abc-123",
            "Fox Session",
            "user",
            "2025-06-15T12:00:00Z",
        )
        .unwrap();

        let results = idx.search("quick brown fox", 10).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].message_id, "msg-abc-123");
        assert_eq!(results[0].session_id, "session-42");
        assert_eq!(results[0].session_title, "Fox Session");
    }

    #[test]
    fn test_search_score_is_negative_rank() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "Rust language is fast and safe",
            "s1",
            "m1",
            "Rust Chat",
            "assistant",
            "2025-03-01T00:00:00Z",
        )
        .unwrap();

        let results = idx.search("Rust", 10).unwrap();
        assert_eq!(results.len(), 1);
        assert!(
            results[0].score < 0.0,
            "expected negative score from FTS5 rank, got {}",
            results[0].score
        );
    }

    #[test]
    fn test_update_session_title_propagates_to_search() {
        let idx = SearchIndex::open_in_memory().unwrap();

        // Index two messages in the same session with old title
        idx.index_message(
            "hello world",
            "s1",
            "m1",
            "Old Title",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();
        idx.index_message(
            "goodbye world",
            "s1",
            "m2",
            "Old Title",
            "assistant",
            "2025-01-01T00:01:00Z",
        )
        .unwrap();

        // Verify old title
        let results = idx.search("hello", 10).unwrap();
        assert_eq!(results[0].session_title, "Old Title");

        // Update session title
        idx.update_session_title("s1", "New Title").unwrap();

        // Verify both messages have new title
        let results = idx.search("hello", 10).unwrap();
        assert_eq!(results[0].session_title, "New Title");

        let results = idx.search("goodbye", 10).unwrap();
        assert_eq!(results[0].session_title, "New Title");
    }

    #[test]
    fn test_search_no_results() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "Rust is a systems programming language",
            "s1",
            "m1",
            "Rust Chat",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();

        let results = idx.search("javascript", 10).unwrap();
        assert!(results.is_empty());
    }

    #[test]
    fn test_mark_session_deleted_idempotent() {
        let idx = SearchIndex::open_in_memory().unwrap();
        idx.index_message(
            "some content",
            "s1",
            "m1",
            "Title",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();

        // Mark deleted twice — INSERT OR IGNORE should not error.
        idx.mark_session_deleted("s1").unwrap();
        idx.mark_session_deleted("s1").unwrap();

        let results = idx.search("content", 10).unwrap();
        assert!(results.is_empty());
    }

    #[test]
    fn test_update_session_title_no_messages() {
        let idx = SearchIndex::open_in_memory().unwrap();
        // Updating title for a session with no indexed messages should be a no-op.
        idx.update_session_title("nonexistent-session", "New Title")
            .unwrap();
    }

    #[test]
    fn test_search_result_role_round_trip() {
        let idx = SearchIndex::open_in_memory().unwrap();

        let roles = [
            ("user", MessageRole::User),
            ("assistant", MessageRole::Assistant),
            ("system", MessageRole::System),
            ("tool", MessageRole::Tool),
            ("unspecified", MessageRole::Unspecified),
        ];

        for (i, (role_str, _)) in roles.iter().enumerate() {
            // Each message needs a unique searchable term.
            let content = format!("unique_role_test_{i} conversation");
            idx.index_message(
                &content,
                "s1",
                &format!("m{i}"),
                "Roles",
                role_str,
                "2025-01-01T00:00:00Z",
            )
            .unwrap();
        }

        for (i, (_, expected_role)) in roles.iter().enumerate() {
            let query = format!("unique_role_test_{i}");
            let results = idx.search(&query, 10).unwrap();
            assert_eq!(results.len(), 1, "expected 1 result for {query}");
            assert_eq!(
                results[0].role, *expected_role,
                "role mismatch for query {query}: got {:?}, expected {:?}",
                results[0].role, expected_role
            );
        }
    }

    #[test]
    fn test_search_result_created_at_fidelity() {
        let idx = SearchIndex::open_in_memory().unwrap();
        let ts = "2025-06-15T14:30:00Z";
        idx.index_message("timestamp fidelity test", "s1", "m1", "Title", "user", ts)
            .unwrap();

        let results = idx.search("timestamp fidelity", 10).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(
            results[0].created_at.to_rfc3339(),
            "2025-06-15T14:30:00+00:00",
            "created_at should survive the index→search round-trip"
        );
    }

    #[test]
    fn test_search_multiple_results_ordering() {
        let idx = SearchIndex::open_in_memory().unwrap();

        // Index messages where "Rust" appears with varying frequency
        // to create different FTS5 relevance scores.
        idx.index_message(
            "Rust Rust Rust is incredibly fast and safe",
            "s1",
            "m1",
            "Heavy Rust",
            "user",
            "2025-01-01T00:00:00Z",
        )
        .unwrap();
        idx.index_message(
            "I heard Rust is good",
            "s2",
            "m2",
            "Light Rust",
            "user",
            "2025-01-02T00:00:00Z",
        )
        .unwrap();

        let results = idx.search("Rust", 10).unwrap();
        assert_eq!(results.len(), 2, "both messages should match");
        // FTS5 rank is negative; more relevant = more negative score.
        assert!(
            results[0].score <= results[1].score,
            "results should be ordered by FTS5 rank (most relevant first): {:?} vs {:?}",
            results[0].score,
            results[1].score
        );
    }
}
