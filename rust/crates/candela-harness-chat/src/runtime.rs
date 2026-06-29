//! Chat runtime stub.

use candela_core::harness::{ChatEvent, HarnessConfig, UsageSummary};
use candela_harness_storage::Database;
use tracing::info;

/// The chat runtime manages active conversations.
pub struct ChatRuntime {
    _config: HarnessConfig,
    _db: Database,
}

impl ChatRuntime {
    /// Create a new chat runtime.
    pub fn new(config: HarnessConfig, db: Database) -> Self {
        Self {
            _config: config,
            _db: db,
        }
    }

    /// Send a message and stream the response.
    ///
    /// Returns the stream ID. Events will be sent via the notification callback.
    pub async fn send_message(
        &self,
        session_id: &str,
        content: &str,
        on_event: impl Fn(ChatEvent) + Send + 'static,
    ) -> Result<String, candela_core::harness::HarnessError> {
        info!(session_id, content_len = content.len(), "chat.send");

        let stream_id = uuid::Uuid::new_v4().to_string();

        // TODO: Implement actual LLM call via Candela proxy
        // For now, echo back a stub response
        on_event(ChatEvent::Chunk {
            stream_id: stream_id.clone(),
            delta: format!("Echo: {content}"),
        });

        on_event(ChatEvent::Done {
            stream_id: stream_id.clone(),
            usage: UsageSummary::default(),
        });

        Ok(stream_id)
    }
}
