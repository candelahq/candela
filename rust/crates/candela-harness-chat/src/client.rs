//! LLM model client — streams chat completions via SSE.

use candela_core::harness::Message;
use candela_core::harness::{
    ChatEvent, ChatEventEvent, ChunkEvent, DoneEvent, HarnessError, UsageSummary,
};
use futures_core::Stream;
use pin_project_lite::pin_project;
use reqwest::Client;
use serde::Deserialize;
use std::pin::Pin;
use std::task::{Context, Poll};
use tracing::{debug, warn};

/// Client for streaming chat completions from an OpenAI-compatible API.
pub struct ModelClient {
    http: Client,
    base_url: String,
}

impl ModelClient {
    /// Create a new model client.
    ///
    /// `base_url` should be the base URL for the API, e.g.:
    /// - Direct: `https://generativelanguage.googleapis.com/v1beta/openai`
    /// - Via proxy: `http://localhost:8080/proxy/openai`
    pub fn new(base_url: &str) -> Self {
        Self {
            http: Client::new(),
            base_url: base_url.trim_end_matches('/').to_string(),
        }
    }

    /// Stream a chat completion. Returns an async stream of ChatEvents.
    pub async fn stream_chat(
        &self,
        messages: &[Message],
        model: &str,
        stream_id: &str,
    ) -> Result<impl Stream<Item = Result<ChatEvent, HarnessError>>, HarnessError> {
        let url = format!("{}/v1/chat/completions", self.base_url);

        // Build OpenAI-compatible message array
        let api_messages: Vec<serde_json::Value> = messages
            .iter()
            .map(|m| {
                serde_json::json!({
                    "role": role_to_api(&m.role),
                    "content": &m.content,
                })
            })
            .collect();

        let body = serde_json::json!({
            "model": model,
            "messages": api_messages,
            "stream": true,
        });

        debug!(url = %url, model = %model, msg_count = messages.len(), "streaming chat");

        // Resolve API key from env
        let api_key = resolve_api_key().ok_or_else(|| {
            HarnessError::Transport(
                "No API key found. Set GEMINI_API_KEY, OPENAI_API_KEY, or CANDELA_API_KEY"
                    .to_string(),
            )
        })?;

        let resp = self
            .http
            .post(&url)
            .bearer_auth(&api_key)
            .json(&body)
            .send()
            .await
            .map_err(|e| HarnessError::Transport(e.to_string()))?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body_text = resp
                .text()
                .await
                .unwrap_or_else(|_| "failed to read body".to_string());
            return Err(HarnessError::Transport(format!(
                "API returned {status}: {body_text}"
            )));
        }

        let byte_stream = resp.bytes_stream();
        Ok(SseStream::new(byte_stream, stream_id.to_string()))
    }
}

/// Resolve an API key from environment variables.
pub(crate) fn resolve_api_key() -> Option<String> {
    std::env::var("CANDELA_API_KEY")
        .or_else(|_| std::env::var("GEMINI_API_KEY"))
        .or_else(|_| std::env::var("OPENAI_API_KEY"))
        .ok()
}

/// Map our MessageRole to the OpenAI API role string.
fn role_to_api(role: &candela_core::harness::MessageRole) -> &'static str {
    use candela_core::harness::MessageRole;
    match role {
        MessageRole::User => "user",
        MessageRole::Assistant => "assistant",
        MessageRole::System => "system",
        MessageRole::Tool => "tool",
        MessageRole::Unspecified => "user",
    }
}

// --- SSE stream parsing ---

/// Partial OpenAI chat completion chunk.
#[derive(Debug, Deserialize)]
struct ChatCompletionChunk {
    choices: Vec<ChunkChoice>,
    usage: Option<ChunkUsage>,
}

#[derive(Debug, Deserialize)]
struct ChunkChoice {
    delta: ChunkDelta,
}

#[derive(Debug, Deserialize)]
struct ChunkDelta {
    content: Option<String>,
}

#[derive(Debug, Deserialize)]
struct ChunkUsage {
    prompt_tokens: Option<i64>,
    completion_tokens: Option<i64>,
    total_tokens: Option<i64>,
}

pin_project! {
    /// Transforms a raw byte stream into ChatEvent items by parsing SSE `data:` lines.
    struct SseStream<S> {
        #[pin]
        inner: S,
        stream_id: String,
        buffer: String,
    }
}

impl<S> SseStream<S> {
    fn new(inner: S, stream_id: String) -> Self {
        Self {
            inner,
            stream_id,
            buffer: String::new(),
        }
    }
}

impl<S, E> Stream for SseStream<S>
where
    S: Stream<Item = Result<bytes::Bytes, E>>,
    E: std::fmt::Display,
{
    type Item = Result<ChatEvent, HarnessError>;

    fn poll_next(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        let mut this = self.project();

        loop {
            // Try to extract a complete line from the buffer.
            if let Some(pos) = this.buffer.find('\n') {
                let line = this.buffer.drain(..=pos).collect::<String>();
                let line = line.trim();

                if line.is_empty() {
                    continue;
                }

                // SSE: data: [DONE]
                if line == "data: [DONE]" {
                    return Poll::Ready(Some(Ok(ChatEvent {
                        stream_id: this.stream_id.clone(),
                        event: Some(ChatEventEvent::Done(Box::new(DoneEvent { usage: None }))),
                    })));
                }

                // SSE: data: {...}
                if let Some(json_str) = line.strip_prefix("data: ") {
                    match serde_json::from_str::<ChatCompletionChunk>(json_str) {
                        Ok(chunk) => {
                            // Extract content delta
                            if let Some(choice) = chunk.choices.first()
                                && let Some(content) = &choice.delta.content
                                && !content.is_empty()
                            {
                                return Poll::Ready(Some(Ok(ChatEvent {
                                    stream_id: this.stream_id.clone(),
                                    event: Some(ChatEventEvent::Chunk(Box::new(ChunkEvent {
                                        delta: content.clone(),
                                    }))),
                                })));
                            }

                            // Check for usage in final chunk
                            if let Some(usage) = chunk.usage {
                                return Poll::Ready(Some(Ok(ChatEvent {
                                    stream_id: this.stream_id.clone(),
                                    event: Some(ChatEventEvent::Done(Box::new(DoneEvent {
                                        usage: Some(Box::new(UsageSummary {
                                            prompt_tokens: usage.prompt_tokens.unwrap_or(0),
                                            completion_tokens: usage.completion_tokens.unwrap_or(0),
                                            total_tokens: usage.total_tokens.unwrap_or(0),
                                            total_cost_usd: 0.0,
                                            model: String::new(),
                                        })),
                                    }))),
                                })));
                            }

                            // Empty delta (e.g. role-only chunk) — skip
                            continue;
                        }
                        Err(e) => {
                            warn!(json = %json_str, error = %e, "failed to parse SSE chunk");
                            continue;
                        }
                    }
                }

                // Non-data SSE lines (event:, id:, retry:) — skip
                continue;
            }

            // No complete line in buffer — poll for more bytes.
            match this.inner.as_mut().poll_next(cx) {
                Poll::Ready(Some(Ok(bytes))) => {
                    this.buffer.push_str(&String::from_utf8_lossy(&bytes));
                }
                Poll::Ready(Some(Err(e))) => {
                    return Poll::Ready(Some(Err(HarnessError::Transport(format!(
                        "stream error: {}",
                        e
                    )))));
                }
                Poll::Ready(None) => {
                    // Stream ended. If there's remaining data in the buffer, try to process it.
                    if !this.buffer.is_empty() {
                        let remaining = std::mem::take(this.buffer);
                        let remaining = remaining.trim();
                        if remaining == "data: [DONE]" {
                            return Poll::Ready(Some(Ok(ChatEvent {
                                stream_id: this.stream_id.clone(),
                                event: Some(ChatEventEvent::Done(Box::new(DoneEvent {
                                    usage: None,
                                }))),
                            })));
                        }
                    }
                    return Poll::Ready(None);
                }
                Poll::Pending => return Poll::Pending,
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio_stream::StreamExt;

    #[test]
    fn test_role_to_api() {
        use candela_core::harness::MessageRole;
        assert_eq!(role_to_api(&MessageRole::User), "user");
        assert_eq!(role_to_api(&MessageRole::Assistant), "assistant");
        assert_eq!(role_to_api(&MessageRole::System), "system");
        assert_eq!(role_to_api(&MessageRole::Tool), "tool");
        assert_eq!(role_to_api(&MessageRole::Unspecified), "user");
    }

    #[test]
    fn test_resolve_api_key_missing() {
        // This test just verifies the function doesn't panic.
        // It may or may not return Some depending on env.
        let _ = resolve_api_key();
    }

    #[tokio::test]
    async fn test_sse_stream_parses_chunks() {
        use tokio_stream::iter;

        let sse_data = "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n\
                        data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n\
                        data: [DONE]\n\n";

        let chunks = vec![Ok::<_, std::io::Error>(bytes::Bytes::from(sse_data))];
        let byte_stream = iter(chunks);
        let mut stream = SseStream::new(byte_stream, "test-stream".to_string());

        let mut events = Vec::new();
        while let Some(item) = stream.next().await {
            events.push(item.unwrap());
        }

        assert_eq!(events.len(), 3); // Hello, " world", Done
        match &events[0] {
            ChatEvent {
                event: Some(ChatEventEvent::Chunk(c)),
                ..
            } => assert_eq!(c.delta, "Hello"),
            other => panic!("expected Chunk, got {other:?}"),
        }
        match &events[1] {
            ChatEvent {
                event: Some(ChatEventEvent::Chunk(c)),
                ..
            } => assert_eq!(c.delta, " world"),
            other => panic!("expected Chunk, got {other:?}"),
        }
        assert!(matches!(
            &events[2],
            ChatEvent {
                event: Some(ChatEventEvent::Done(_)),
                ..
            }
        ));
    }

    #[tokio::test]
    async fn test_sse_stream_handles_split_bytes() {
        use tokio_stream::iter;

        // SSE data split across two byte chunks
        let chunks = vec![
            Ok::<_, std::io::Error>(bytes::Bytes::from("data: {\"choices\":[{\"del")),
            Ok(bytes::Bytes::from(
                "ta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n",
            )),
        ];
        let byte_stream = iter(chunks);
        let mut stream = SseStream::new(byte_stream, "s1".to_string());

        let mut events = Vec::new();
        while let Some(item) = stream.next().await {
            events.push(item.unwrap());
        }

        assert_eq!(events.len(), 2);
        match &events[0] {
            ChatEvent {
                event: Some(ChatEventEvent::Chunk(c)),
                ..
            } => assert_eq!(c.delta, "Hi"),
            other => panic!("expected Chunk, got {other:?}"),
        }
        assert!(matches!(
            &events[1],
            ChatEvent {
                event: Some(ChatEventEvent::Done(_)),
                ..
            }
        ));
    }

    #[tokio::test]
    async fn test_sse_stream_skips_empty_delta() {
        // Role-only chunk (no content field) should be skipped
        let sse_data = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n\
                        data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n\
                        data: [DONE]\n\n";

        let chunks = vec![Ok::<_, std::io::Error>(bytes::Bytes::from(sse_data))];
        let mut stream = SseStream::new(tokio_stream::iter(chunks), "s1".to_string());

        let mut events = Vec::new();
        while let Some(item) = stream.next().await {
            events.push(item.unwrap());
        }

        // Role-only chunk should be skipped, leaving Chunk("Hi") + Done
        assert_eq!(events.len(), 2);
        match &events[0] {
            ChatEvent {
                event: Some(ChatEventEvent::Chunk(c)),
                ..
            } => assert_eq!(c.delta, "Hi"),
            other => panic!("expected Chunk, got {other:?}"),
        }
        assert!(matches!(
            &events[1],
            ChatEvent {
                event: Some(ChatEventEvent::Done(_)),
                ..
            }
        ));
    }

    #[tokio::test]
    async fn test_sse_stream_extracts_usage() {
        use tokio_stream::iter;

        let sse_data = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n\
                        data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n\
                        data: [DONE]\n\n";

        let chunks = vec![Ok::<_, std::io::Error>(bytes::Bytes::from(sse_data))];
        let mut stream = SseStream::new(iter(chunks), "s1".to_string());

        let mut events = Vec::new();
        while let Some(item) = stream.next().await {
            events.push(item.unwrap());
        }

        assert_eq!(events.len(), 3); // Chunk, Done(usage), Done([DONE])
        match &events[1] {
            ChatEvent {
                event: Some(ChatEventEvent::Done(d)),
                ..
            } => {
                let usage = d.usage.as_ref().expect("expected usage");
                assert_eq!(usage.prompt_tokens, 10);
                assert_eq!(usage.completion_tokens, 5);
                assert_eq!(usage.total_tokens, 15);
            }
            other => panic!("expected Done with usage, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_sse_stream_skips_malformed_json() {
        use tokio_stream::iter;

        // Malformed JSON line should be skipped, not cause an error
        let sse_data = "data: {not valid json}\n\n\
                        data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n\
                        data: [DONE]\n\n";

        let chunks = vec![Ok::<_, std::io::Error>(bytes::Bytes::from(sse_data))];
        let mut stream = SseStream::new(iter(chunks), "s1".to_string());

        let mut events = Vec::new();
        while let Some(item) = stream.next().await {
            events.push(item.unwrap());
        }

        // Malformed line skipped, then Chunk + Done
        assert_eq!(events.len(), 2);
        match &events[0] {
            ChatEvent {
                event: Some(ChatEventEvent::Chunk(c)),
                ..
            } => assert_eq!(c.delta, "ok"),
            other => panic!("expected Chunk, got {other:?}"),
        }
        assert!(matches!(
            &events[1],
            ChatEvent {
                event: Some(ChatEventEvent::Done(_)),
                ..
            }
        ));
    }

    #[tokio::test]
    async fn test_sse_stream_propagates_stream_error() {
        use tokio_stream::iter;

        let chunks: Vec<Result<bytes::Bytes, std::io::Error>> = vec![
            Ok(bytes::Bytes::from(
                "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
            )),
            Err(std::io::Error::new(
                std::io::ErrorKind::BrokenPipe,
                "pipe broke",
            )),
        ];
        let mut stream = SseStream::new(iter(chunks), "s1".to_string());

        // First event should be a Chunk
        let first = stream.next().await.unwrap();
        assert!(first.is_ok());

        // Second event should be a Transport error
        let second = stream.next().await.unwrap();
        assert!(second.is_err());
        let err = second.unwrap_err();
        assert!(
            matches!(&err, HarnessError::Transport(msg) if msg.contains("pipe broke")),
            "expected Transport error containing 'pipe broke', got: {err:?}"
        );
    }

    #[tokio::test]
    async fn test_sse_stream_empty_stream() {
        use tokio_stream::iter;

        // Completely empty stream — should yield nothing
        let chunks: Vec<Result<bytes::Bytes, std::io::Error>> = vec![];
        let mut stream = SseStream::new(iter(chunks), "s1".to_string());

        let event = stream.next().await;
        assert!(event.is_none(), "empty stream should yield None");
    }
}
