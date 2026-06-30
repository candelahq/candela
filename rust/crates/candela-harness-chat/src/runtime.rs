//! Chat runtime — manages the conversation loop, model calls, and streaming.

use candela_core::harness::{
    ChatEvent, HarnessConfig, HarnessError, MessageRole, UsageSummary, new_message,
};
use candela_harness_storage::{Database, SearchIndex};
use std::sync::{Arc, Mutex};
use tokio_stream::StreamExt;
use tracing::{error, info};

use crate::client::ModelClient;

/// The chat runtime manages active conversations.
pub struct ChatRuntime {
    config: HarnessConfig,
    db: Arc<Mutex<Database>>,
    search: Arc<Mutex<SearchIndex>>,
    client: ModelClient,
}

impl ChatRuntime {
    /// Create a new chat runtime.
    pub fn new(
        config: HarnessConfig,
        db: Arc<Mutex<Database>>,
        search: Arc<Mutex<SearchIndex>>,
    ) -> Self {
        let base_url = config.proxy_url.clone().unwrap_or_else(|| {
            "https://generativelanguage.googleapis.com/v1beta/openai".to_string()
        });

        let client = ModelClient::new(&base_url);

        Self {
            config,
            db,
            search,
            client,
        }
    }

    /// Send a message and stream the response.
    ///
    /// Returns the stream ID. Events will be sent via the `on_event` callback
    /// as they arrive from the model.
    pub async fn send_message(
        &self,
        session_id: &str,
        content: &str,
        on_event: impl Fn(ChatEvent) + Send + 'static,
    ) -> Result<String, HarnessError> {
        let stream_id = uuid::Uuid::new_v4().to_string();
        info!(
            session_id,
            stream_id,
            content_len = content.len(),
            "chat.send"
        );

        // 1. Emit status
        on_event(ChatEvent::Status {
            stream_id: stream_id.clone(),
            text: "Thinking...".to_string(),
            agent: None,
        });

        // 2. Store user message
        let user_msg = new_message(session_id, MessageRole::User, content);
        self.db.lock().unwrap().insert_message(&user_msg)?;

        // 3. Load conversation history
        let history = self.db.lock().unwrap().get_messages(session_id, 100)?;

        // 4. Stream from model
        let model = &self.config.model;
        let mut full_response = String::new();
        let mut usage = UsageSummary::default();

        let stream = self.client.stream_chat(&history, model, &stream_id).await?;
        tokio::pin!(stream);

        while let Some(event) = stream.next().await {
            match event {
                Ok(ChatEvent::Chunk { delta, .. }) => {
                    full_response.push_str(&delta);
                    on_event(ChatEvent::Chunk {
                        stream_id: stream_id.clone(),
                        delta,
                    });
                }
                Ok(ChatEvent::Done { usage: u, .. }) => {
                    usage = u;
                    usage.model = model.to_string();
                }
                Ok(other) => on_event(other),
                Err(e) => {
                    error!(error = %e, "stream error");
                    on_event(ChatEvent::Error {
                        stream_id: stream_id.clone(),
                        message: e.to_string(),
                    });
                    return Err(e);
                }
            }
        }

        // 5. Store assistant message
        if !full_response.is_empty() {
            let mut assistant_msg = new_message(session_id, MessageRole::Assistant, &full_response);
            assistant_msg.model = Some(model.to_string());
            assistant_msg.token_count = Some(usage.total_tokens as i32);
            assistant_msg.cost_usd = if usage.total_cost_usd > 0.0 {
                Some(usage.total_cost_usd)
            } else {
                None
            };
            self.db.lock().unwrap().insert_message(&assistant_msg)?;

            // 6. Index in FTS for search
            let _ = self.search.lock().unwrap().index_message(
                &full_response,
                session_id,
                &assistant_msg.id,
                "", // session title — we don't have it here
                "assistant",
                &assistant_msg.created_at.to_rfc3339(),
            );
        }

        // 7. Emit Done
        on_event(ChatEvent::Done {
            stream_id: stream_id.clone(),
            usage,
        });

        Ok(stream_id.clone())
    }
}
