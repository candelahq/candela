//! Domain types, configuration, and errors for the candela-harness IDE sidecar.
//!
//! These types were originally in the standalone `harness-core` crate and have
//! been merged into `candela-core` so that every harness crate depends on a
//! single shared type library.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

// ── Configuration ──

/// Top-level harness configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HarnessConfig {
    /// Directory for session storage.
    pub save_dir: std::path::PathBuf,
    /// Candela proxy URL for model routing.
    pub proxy_url: Option<String>,
    /// Default model identifier.
    pub model: String,
    /// Budget limit in USD. 0 = unlimited.
    // TODO: Consider using integer micro-units (i64 microdollars) instead of f64
    // to avoid floating-point rounding in budget calculations.
    pub budget_limit_usd: f64,
    /// Device ID for cross-device sync.
    pub device_id: String,
    /// Enable subagent delegation.
    pub enable_subagents: bool,
    /// Transport mode.
    pub transport: TransportMode,
    /// HTTP port (for daemon mode).
    pub http_port: u16,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum TransportMode {
    #[default]
    Stdio,
    Http,
}

impl Default for HarnessConfig {
    fn default() -> Self {
        Self {
            save_dir: std::path::PathBuf::from(".candela/sessions"),
            proxy_url: None,
            model: "gemini-2.0-flash".to_string(),
            budget_limit_usd: 5.0,
            device_id: String::new(),
            enable_subagents: true,
            transport: TransportMode::Stdio,
            http_port: 8200,
        }
    }
}

// ── Errors ──

/// Harness-specific error type.
#[derive(Debug, thiserror::Error)]
pub enum HarnessError {
    #[error("storage error: {0}")]
    Storage(String),

    #[error("serialization error: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("session not found: {0}")]
    SessionNotFound(String),

    #[error("budget exceeded: used ${used:.4} of ${limit:.4} limit")]
    BudgetExceeded { used: f64, limit: f64 },

    #[error("transport error: {0}")]
    Transport(String),

    #[error("{0}")]
    Other(#[from] anyhow::Error),
}

// ── Domain Types ──

/// A chat session (conversation).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    pub id: String,
    pub title: String,
    pub model: String,
    pub message_count: i32,
    pub total_tokens: i64,
    pub total_cost_usd: f64,
    pub device_id: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub deleted_at: Option<DateTime<Utc>>,
}

impl Session {
    pub fn new(model: &str, device_id: &str) -> Self {
        let now = Utc::now();
        let id = uuid::Uuid::new_v4().to_string();
        Self {
            id,
            title: "New Chat".to_string(),
            model: model.to_string(),
            message_count: 0,
            total_tokens: 0,
            total_cost_usd: 0.0,
            device_id: device_id.to_string(),
            created_at: now,
            updated_at: now,
            deleted_at: None,
        }
    }
}

/// A single chat message.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: i64,
    pub session_id: String,
    pub role: MessageRole,
    pub content: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cost_usd: Option<f64>,
    pub created_at: DateTime<Utc>,
}

/// Message roles.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum MessageRole {
    User,
    Assistant,
    System,
    Tool,
}

/// Streaming chat events sent as JSON-RPC notifications.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ChatEvent {
    /// Text delta from the model.
    Chunk { stream_id: String, delta: String },
    /// Model wants to call a tool.
    ToolCall {
        stream_id: String,
        call_id: String,
        tool: String,
        args: serde_json::Value,
        requires_approval: bool,
    },
    /// Status update (e.g., "Analyzing codebase...").
    Status {
        stream_id: String,
        text: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        agent: Option<String>,
    },
    /// Stream completed.
    Done {
        stream_id: String,
        usage: UsageSummary,
    },
    /// Stream error.
    Error { stream_id: String, message: String },
}

/// Token/cost usage summary.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UsageSummary {
    pub prompt_tokens: i64,
    pub completion_tokens: i64,
    pub total_tokens: i64,
    pub total_cost_usd: f64,
}

/// Search result from FTS5 index.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SearchResult {
    pub session_id: String,
    pub session_title: String,
    pub message_preview: String,
    pub role: MessageRole,
    pub score: f64,
    pub created_at: DateTime<Utc>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn session_new_has_valid_defaults() {
        let session = Session::new("gemini-2.0-flash", "device-1");
        assert_eq!(session.title, "New Chat");
        assert_eq!(session.model, "gemini-2.0-flash");
        assert_eq!(session.message_count, 0);
        assert!(session.deleted_at.is_none());
        assert!(!session.id.is_empty());
    }

    #[test]
    fn session_json_round_trip() {
        let session = Session::new("test-model", "dev-1");
        let json = serde_json::to_string(&session).unwrap();
        let restored: Session = serde_json::from_str(&json).unwrap();
        assert_eq!(restored.id, session.id);
        assert_eq!(restored.model, "test-model");
    }

    #[test]
    fn chat_event_serde_round_trip() {
        let event = ChatEvent::Done {
            stream_id: "s1".into(),
            usage: UsageSummary::default(),
        };
        let json = serde_json::to_string(&event).unwrap();
        let restored: ChatEvent = serde_json::from_str(&json).unwrap();
        assert!(matches!(restored, ChatEvent::Done { .. }));
    }

    #[test]
    fn message_role_serde() {
        let json = serde_json::to_string(&MessageRole::Assistant).unwrap();
        assert_eq!(json, "\"assistant\"");
        let restored: MessageRole = serde_json::from_str(&json).unwrap();
        assert_eq!(restored, MessageRole::Assistant);
    }

    #[test]
    fn harness_config_default() {
        let config = HarnessConfig::default();
        assert_eq!(config.model, "gemini-2.0-flash");
        assert_eq!(config.http_port, 8200);
    }
}
