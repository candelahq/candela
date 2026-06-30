//! Domain types, configuration, and errors for the candela-harness IDE sidecar.
//!
//! Session, Message, MessageRole, SearchResult, and UsageSummary are
//! re-exported from `candela-types` (generated from candela-protos).
//! HarnessConfig, HarnessError, TransportMode, and ChatEvent remain
//! harness-specific and are defined here.

use serde::{Deserialize, Serialize};

// ── Re-exported proto-generated types ──

pub use candela_types::chat::UsageSummary;
pub use candela_types::session::{Message, MessageRole, SearchResult, Session};

// ── Session constructor ──

/// Extension methods for creating new sessions.
pub fn new_session(model: &str, device_id: &str) -> Session {
    let now = chrono::Utc::now();
    let id = uuid::Uuid::new_v4().to_string();
    Session {
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

/// Create a new user message for a session.
pub fn new_message(session_id: &str, role: MessageRole, content: &str) -> Message {
    Message {
        id: uuid::Uuid::new_v4().to_string(),
        session_id: session_id.to_string(),
        role,
        content: content.to_string(),
        created_at: chrono::Utc::now(),
        ..Default::default()
    }
}

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

// ── Streaming Chat Events (wire format) ──

/// Streaming chat events sent as JSON-RPC notifications.
///
/// This type defines the JSON-RPC wire format for streaming. It intentionally
/// differs from the proto-generated `ChatEvent` (which uses struct + oneof)
/// because the JSON-RPC protocol needs a flat tagged enum for ergonomic
/// serialization over stdio.
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn session_new_has_valid_defaults() {
        let session = new_session("gemini-2.0-flash", "device-1");
        assert_eq!(session.title, "New Chat");
        assert_eq!(session.model, "gemini-2.0-flash");
        assert_eq!(session.message_count, 0);
        assert!(session.deleted_at.is_none());
        assert!(!session.id.is_empty());
    }

    #[test]
    fn session_json_round_trip() {
        let session = new_session("test-model", "dev-1");
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
    fn message_new_has_uuid() {
        let msg = new_message("sess-1", MessageRole::User, "hello");
        assert!(!msg.id.is_empty());
        assert_eq!(msg.session_id, "sess-1");
        assert_eq!(msg.content, "hello");
    }

    #[test]
    fn harness_config_default() {
        let config = HarnessConfig::default();
        assert_eq!(config.model, "gemini-2.0-flash");
        assert_eq!(config.http_port, 8200);
    }

    #[test]
    fn test_chat_event_all_variants_serde() {
        let events = vec![
            ChatEvent::Chunk {
                stream_id: "s1".into(),
                delta: "Hello".into(),
            },
            ChatEvent::ToolCall {
                stream_id: "s1".into(),
                call_id: "c1".into(),
                tool: "read_file".into(),
                args: serde_json::json!({"path": "/tmp/test"}),
                requires_approval: true,
            },
            ChatEvent::Status {
                stream_id: "s1".into(),
                text: "Analyzing...".into(),
                agent: Some("sub-1".into()),
            },
            ChatEvent::Error {
                stream_id: "s1".into(),
                message: "rate limit".into(),
            },
        ];

        for event in &events {
            let json = serde_json::to_string(event).unwrap();
            let restored: ChatEvent = serde_json::from_str(&json).unwrap();
            // Verify the variant tag survived the round-trip.
            let tag = |e: &ChatEvent| match e {
                ChatEvent::Chunk { .. } => "chunk",
                ChatEvent::ToolCall { .. } => "tool_call",
                ChatEvent::Status { .. } => "status",
                ChatEvent::Done { .. } => "done",
                ChatEvent::Error { .. } => "error",
            };
            assert_eq!(
                tag(event),
                tag(&restored),
                "variant mismatch for json: {json}"
            );
        }
    }

    #[test]
    fn test_new_message_sets_defaults() {
        let msg = new_message("sess-1", MessageRole::Assistant, "hi");
        // model should be None (from Default).
        assert_eq!(msg.model, None);
        // token_count should be None.
        assert_eq!(msg.token_count, None);
        // sequence defaults to 0 (DB auto-assigns on insert).
        assert_eq!(msg.sequence, 0);
        // cost_usd should be None.
        assert_eq!(msg.cost_usd, None);
    }
}
