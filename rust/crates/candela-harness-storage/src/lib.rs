//! SQLite storage for sessions and messages with FTS5 search.

pub mod db;
pub mod search;

pub use db::Database;
pub use search::SearchIndex;
