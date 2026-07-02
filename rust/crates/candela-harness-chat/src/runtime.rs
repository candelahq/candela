//! Chat runtime — manages the conversation loop, model calls, and streaming.

use candela_core::harness::{
    ChatEvent, ChatEventEvent, ChunkEvent, DoneEvent, ErrorEvent, HarnessConfig, HarnessError,
    MessageRole, StatusEvent, UsageSummary, new_message,
};
use candela_harness_storage::{Database, SearchIndex};
use std::sync::{Arc, Mutex};
use tokio::sync::RwLock;
use tokio_stream::StreamExt;
use tracing::{debug, error, info, warn};

use crate::client::ModelClient;

/// Timeout for LLM title generation calls.
const TITLE_LLM_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(5);

/// The chat runtime manages active conversations.
pub struct ChatRuntime {
    config: HarnessConfig,
    db: Arc<Mutex<Database>>,
    search: Arc<Mutex<SearchIndex>>,
    client: ModelClient,
    /// Ranked list of models for title generation, sorted cheapest-first.
    /// Empty means not yet resolved (will query API on first use).
    /// Failed models are removed; when empty, re-resolves from API.
    title_model_candidates: RwLock<Vec<String>>,
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
            title_model_candidates: RwLock::new(Vec::new()),
        }
    }

    /// Manually edit a session's title.
    pub fn edit_session_title(&self, session_id: &str, title: &str) -> Result<(), HarnessError> {
        self.db
            .lock()
            .unwrap()
            .update_session_title(session_id, title)?;

        // Keep search index in sync
        let _ = self
            .search
            .lock()
            .unwrap()
            .update_session_title(session_id, title);

        Ok(())
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
        on_event(ChatEvent {
            stream_id: stream_id.clone(),
            event: Some(ChatEventEvent::Status(Box::new(StatusEvent {
                text: "Thinking...".to_string(),
                agent: None,
            }))),
        });

        // 2. Budget check — reject before incurring cost
        let budget_limit = self.config.budget_limit_usd;
        if budget_limit > 0.0 {
            let session = self.db.lock().unwrap().get_session(session_id)?;
            if session.total_cost_usd >= budget_limit {
                return Err(HarnessError::BudgetExceeded {
                    used: session.total_cost_usd,
                    limit: budget_limit,
                });
            }
        }

        // 3. Store user message
        let user_msg = new_message(session_id, MessageRole::User, content);
        self.db.lock().unwrap().insert_message(&user_msg)?;

        // 4. Load conversation history
        let history = self.db.lock().unwrap().get_messages(session_id, 100)?;

        // 5. Stream from model
        let model = &self.config.model;
        let mut full_response = String::new();
        let mut usage = UsageSummary::default();

        let stream = self.client.stream_chat(&history, model, &stream_id).await?;
        tokio::pin!(stream);

        while let Some(event) = stream.next().await {
            match event {
                Ok(ChatEvent {
                    event: Some(ChatEventEvent::Chunk(ref c)),
                    ..
                }) => {
                    full_response.push_str(&c.delta);
                    on_event(ChatEvent {
                        stream_id: stream_id.clone(),
                        event: Some(ChatEventEvent::Chunk(Box::new(ChunkEvent {
                            delta: c.delta.clone(),
                        }))),
                    });
                }
                Ok(ChatEvent {
                    event: Some(ChatEventEvent::Done(ref d)),
                    ..
                }) => {
                    if let Some(ref u) = d.usage {
                        usage = *u.clone();
                    }
                    usage.model = model.to_string();
                }
                Ok(other) => on_event(other),
                Err(e) => {
                    error!(error = %e, "stream error");
                    on_event(ChatEvent {
                        stream_id: stream_id.clone(),
                        event: Some(ChatEventEvent::Error(Box::new(ErrorEvent {
                            message: e.to_string(),
                            code: None,
                        }))),
                    });
                    return Err(e);
                }
            }
        }

        // 6. Store assistant message
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

            // 7. Index in FTS for search
            let _ = self.search.lock().unwrap().index_message(
                &full_response,
                session_id,
                &assistant_msg.id,
                "", // title populated asynchronously by auto-titling
                "assistant",
                &assistant_msg.created_at.to_rfc3339(),
            );
        }

        // 8. Emit Done immediately — don't block UI on title generation
        on_event(ChatEvent {
            stream_id: stream_id.clone(),
            event: Some(ChatEventEvent::Done(Box::new(DoneEvent {
                usage: Some(Box::new(usage)),
            }))),
        });

        // 9. Auto-title in background (non-blocking)
        if !full_response.is_empty() {
            let user_content = content.to_string();
            // auto_title_session borrows &self, so we call it directly but after Done
            self.auto_title_session(session_id, &user_content, &full_response)
                .await;
        }

        Ok(stream_id.clone())
    }

    /// Auto-title a session if it still has the default "New Chat" title.
    ///
    /// Modes (from `config.auto_title_model`):
    /// - `Some("truncate")` (default): first ~80 chars, word-boundary aware
    /// - `Some("keywords")`: local keyword extraction, no API call
    /// - `Some("auto")`: resolve cheapest model via API, try in price order
    /// - `Some("model-name")`: use that specific model for LLM summarization
    /// - `None`: disabled, no auto-titling
    async fn auto_title_session(
        &self,
        session_id: &str,
        user_content: &str,
        _assistant_content: &str,
    ) {
        let session = match self.db.lock().unwrap().get_session(session_id) {
            Ok(s) => s,
            Err(_) => return,
        };

        if session.title != "New Chat" {
            return;
        }

        let title = match self.config.auto_title_model.as_deref() {
            None => return, // Disabled
            Some("truncate") => truncate_to_title(user_content),
            Some("keywords") => extract_title_keywords(user_content),
            Some("auto") => {
                // Try models in price order until one succeeds
                self.generate_title_with_fallback(user_content)
                    .await
                    .unwrap_or_else(|| extract_title_keywords(user_content))
            }
            Some(model) => {
                // Explicit model name
                match self.generate_title_via_llm(user_content, model).await {
                    Some(t) => t,
                    None => extract_title_keywords(user_content),
                }
            }
        };

        // Re-check title before update to avoid overwriting a manual rename
        // that happened while we were generating the title.
        let db = self.db.lock().unwrap();
        match db.get_session(session_id) {
            Ok(current) if current.title == "New Chat" => {
                if let Err(e) = db.update_session_title(session_id, &title) {
                    error!(error = %e, session_id, "failed to auto-title session");
                } else {
                    info!(session_id, title_len = title.len(), "auto-titled session");
                    // Keep search index in sync
                    let _ = self
                        .search
                        .lock()
                        .unwrap()
                        .update_session_title(session_id, &title);
                }
            }
            _ => {
                debug!(
                    session_id,
                    "session title already changed, skipping auto-title"
                );
            }
        }
    }

    /// Try models from the ranked candidate list until one succeeds.
    ///
    /// If the list is empty, resolves from the API first.
    /// Failed models are removed from the list so they won't be retried.
    async fn generate_title_with_fallback(&self, user_content: &str) -> Option<String> {
        // Ensure we have candidates
        {
            let candidates = self.title_model_candidates.read().await;
            if candidates.is_empty() {
                drop(candidates);
                let resolved = self.resolve_model_candidates().await;
                if !resolved.is_empty() {
                    let mut candidates = self.title_model_candidates.write().await;
                    *candidates = resolved;
                }
            }
        }

        // Try each candidate in order
        let models: Vec<String> = self.title_model_candidates.read().await.clone();
        for model in &models {
            debug!(model, "trying model for title generation");
            match tokio::time::timeout(
                TITLE_LLM_TIMEOUT,
                self.generate_title_via_llm(user_content, model),
            )
            .await
            {
                Ok(Some(title)) => return Some(title),
                Ok(None) => {
                    // LLM returned empty/error — remove this model
                    warn!(model, "title model failed, removing from candidates");
                    self.remove_model_candidate(model).await;
                }
                Err(_) => {
                    // Timeout — remove this model
                    warn!(model, "title model timed out, removing from candidates");
                    self.remove_model_candidate(model).await;
                }
            }
        }

        // All models failed
        None
    }

    /// Remove a failed model from the candidate list.
    async fn remove_model_candidate(&self, model: &str) {
        let mut candidates = self.title_model_candidates.write().await;
        candidates.retain(|m| m != model);
    }

    /// Resolve ranked model candidates by querying the models.list API.
    ///
    /// Returns models sorted cheapest-first (flash-lite > flash > other).
    /// Returns empty Vec if the API call fails.
    async fn resolve_model_candidates(&self) -> Vec<String> {
        let base_url = self
            .config
            .proxy_url
            .clone()
            .unwrap_or_else(|| "https://generativelanguage.googleapis.com/v1beta".to_string());

        // Strip /openai suffix if present (we need the native endpoint)
        let api_base = base_url
            .trim_end_matches('/')
            .trim_end_matches("/openai")
            .to_string();

        let api_key = crate::client::resolve_api_key();
        let url = match &api_key {
            Some(key) => format!("{api_base}/models?key={key}"),
            None => format!("{api_base}/models"),
        };

        debug!(
            api_base = %api_base,
            has_api_key = api_key.is_some(),
            "resolving model candidates"
        );

        let resp = match tokio::time::timeout(TITLE_LLM_TIMEOUT, reqwest::get(&url)).await {
            Ok(Ok(r)) => r,
            Ok(Err(e)) => {
                warn!(error = %e, "failed to fetch models list for auto-title");
                return Vec::new();
            }
            Err(_) => {
                warn!("models list request timed out");
                return Vec::new();
            }
        };

        let body: serde_json::Value = match resp.json().await {
            Ok(v) => v,
            Err(e) => {
                warn!(error = %e, "failed to parse models list response");
                return Vec::new();
            }
        };

        let Some(models) = body["models"].as_array() else {
            return Vec::new();
        };

        // Filter for models that support generateContent
        let mut candidates: Vec<&str> = models
            .iter()
            .filter(|m| {
                m["supportedGenerationMethods"]
                    .as_array()
                    .is_some_and(|methods| {
                        methods
                            .iter()
                            .any(|v| v.as_str() == Some("generateContent"))
                    })
            })
            .filter_map(|m| m["name"].as_str())
            .filter_map(|name| name.strip_prefix("models/"))
            .collect();

        // Sort: flash-lite first, then flash, then others. Higher version numbers first.
        candidates.sort_by(|a, b| {
            let score = |name: &str| -> i32 {
                if name.contains("flash-lite") {
                    0
                } else if name.contains("flash") {
                    1
                } else {
                    2
                }
            };
            let sa = score(a);
            let sb = score(b);
            if sa != sb {
                sa.cmp(&sb)
            } else {
                // Reverse alphabetical to prefer higher version numbers
                b.cmp(a)
            }
        });

        let result: Vec<String> = candidates.iter().map(|s| s.to_string()).collect();
        info!(
            count = result.len(),
            "resolved model candidates for auto-titling"
        );
        result
    }

    /// Generate a title using a small LLM model.
    async fn generate_title_via_llm(&self, user_content: &str, model: &str) -> Option<String> {
        use candela_core::harness::Message;

        // Truncate input to avoid context length errors on large messages
        let truncated_input: String = user_content.chars().take(1000).collect();

        let prompt = format!(
            "Generate a short title (max 6 words) for a chat that starts with this message. \
             Return ONLY the title, no quotes or punctuation:\n\n{truncated_input}"
        );
        let messages = vec![Message {
            role: MessageRole::User,
            content: prompt,
            ..Default::default()
        }];

        let stream_id = uuid::Uuid::new_v4().to_string();
        let stream = match self.client.stream_chat(&messages, model, &stream_id).await {
            Ok(s) => s,
            Err(e) => {
                error!(error = %e, model, "title generation LLM call failed");
                return None;
            }
        };
        tokio::pin!(stream);

        let mut title = String::new();
        let mut had_error = false;
        while let Some(event) = stream.next().await {
            match event {
                Ok(ChatEvent {
                    event: Some(ChatEventEvent::Chunk(ref c)),
                    ..
                }) => title.push_str(&c.delta),
                Ok(ChatEvent {
                    event: Some(ChatEventEvent::Error(ref e)),
                    ..
                }) => {
                    warn!(error = %e.message, "title stream error");
                    had_error = true;
                    break;
                }
                Err(e) => {
                    warn!(error = %e, "title stream error");
                    had_error = true;
                    break;
                }
                _ => {}
            }
        }

        // If we had an error and got no useful content, return None to fall back
        if had_error && title.trim().is_empty() {
            return None;
        }

        // Trim whitespace and surrounding quotes (LLMs often wrap in quotes)
        let title = title
            .trim()
            .trim_matches('"')
            .trim_matches('\'')
            .trim()
            .to_string();

        if title.is_empty() {
            None
        } else {
            // Clamp to reasonable length
            Some(title.chars().take(80).collect())
        }
    }
}

// ── Title generation helpers ──

/// Return the byte index after the first `max_chars` characters in `s`.
/// If `s` has fewer than `max_chars` characters, returns `s.len()`.
fn char_limit_byte_index(s: &str, max_chars: usize) -> usize {
    s.char_indices()
        .nth(max_chars)
        .map(|(idx, _)| idx)
        .unwrap_or(s.len())
}

/// Truncate content to a reasonable title (first ~80 chars, word-boundary aware).
fn truncate_to_title(content: &str) -> String {
    let content = content.trim();
    if content.chars().count() <= 80 {
        return content.to_string();
    }

    // Truncate at char boundary (80 characters, not bytes)
    let boundary = char_limit_byte_index(content, 80);
    let truncated = &content[..boundary];
    if let Some(last_space) = truncated.rfind(' ')
        && last_space > 20
    {
        return format!("{}…", &content[..last_space]);
    }
    format!("{}…", truncated)
}

/// Common English stop words to filter out for keyword extraction.
const STOP_WORDS: &[&str] = &[
    "a", "an", "the", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had",
    "do", "does", "did", "will", "would", "could", "should", "may", "might", "shall", "can", "i",
    "me", "my", "we", "our", "you", "your", "he", "she", "it", "they", "them", "his", "her", "its",
    "this", "that", "these", "those", "what", "which", "who", "whom", "in", "on", "at", "to",
    "for", "of", "with", "by", "from", "as", "into", "about", "and", "or", "but", "not", "no",
    "if", "so", "just", "also", "very", "really", "how", "when", "where", "why", "all", "each",
    "some", "any", "there", "here", "please", "help", "want", "need", "like", "know", "think",
    "make", "get", "use",
];

/// Extract keywords from content to create a concise title.
///
/// Removes stop words, keeps the most significant words, and title-cases them.
fn extract_title_keywords(content: &str) -> String {
    let content = content.trim();

    // Take the first sentence or first ~200 characters
    let limit = char_limit_byte_index(content, 200);
    let first_chunk = content
        .find(['.', '?', '!'])
        .map(|pos| &content[..pos])
        .unwrap_or(&content[..limit]);

    let words: Vec<&str> = first_chunk
        .split(|c: char| !c.is_alphanumeric() && c != '-' && c != '_' && c != '\'')
        .filter(|w| !w.is_empty())
        .filter(|w| w.len() > 1) // skip single chars
        .filter(|w| !STOP_WORDS.contains(&w.to_lowercase().as_str()))
        .take(6)
        .collect();

    if words.is_empty() {
        return truncate_to_title(content);
    }

    // Title-case each word
    words
        .iter()
        .map(|w| {
            let mut chars = w.chars();
            match chars.next() {
                Some(c) => {
                    let upper: String = c.to_uppercase().collect();
                    format!("{upper}{}", chars.as_str().to_lowercase())
                }
                None => String::new(),
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_truncate_short_content() {
        assert_eq!(truncate_to_title("Hello world"), "Hello world");
    }

    #[test]
    fn test_truncate_exact_80() {
        let content = "a".repeat(80);
        assert_eq!(truncate_to_title(&content), content);
    }

    #[test]
    fn test_truncate_long_with_word_boundary() {
        let content = format!("{} {}", "a".repeat(60), "b".repeat(30));
        let title = truncate_to_title(&content);
        assert!(title.ends_with('…'));
    }

    #[test]
    fn test_truncate_long_no_good_boundary() {
        let content = "a".repeat(120);
        let title = truncate_to_title(&content);
        assert!(title.ends_with('…'));
        // Should be 80 chars + ellipsis (1 char)
        assert_eq!(title.chars().count(), 81);
    }

    #[test]
    fn test_truncate_trims_whitespace() {
        assert_eq!(truncate_to_title("  hello  "), "hello");
    }

    #[test]
    fn test_truncate_multibyte_no_panic() {
        // 27 × '€' = 81 bytes (each '€' is 3 bytes), more than 80
        let content = "€".repeat(27);
        let title = truncate_to_title(&content);
        // Should not panic — must handle multibyte correctly
        assert!(!title.is_empty());
        assert!(title.chars().count() <= 81); // 80 chars + possible ellipsis
    }

    #[test]
    fn test_keywords_basic() {
        let title = extract_title_keywords(
            "Can you help me refactor the authentication middleware to use JWT tokens?",
        );
        assert!(!title.is_empty());
        assert!(!title.contains("you"));
        assert!(!title.contains("help"));
        assert!(!title.contains("me"));
        // Should contain significant words
        assert!(
            title.contains("Refactor") || title.contains("Authentication") || title.contains("Jwt")
        );
    }

    #[test]
    fn test_keywords_max_6_words() {
        let title = extract_title_keywords(
            "Implement distributed caching layer with Redis cluster for session management and rate limiting across microservices",
        );
        let word_count = title.split_whitespace().count();
        assert!(word_count <= 6, "got {word_count} words: {title}");
    }

    #[test]
    fn test_keywords_title_case() {
        let title = extract_title_keywords("debug the failing unit tests");
        for word in title.split_whitespace() {
            assert!(
                word.chars().next().unwrap().is_uppercase(),
                "'{word}' should be title-cased in '{title}'"
            );
        }
    }

    #[test]
    fn test_keywords_empty_after_stop_words() {
        // All stop words — should fall back to truncation
        let title = extract_title_keywords("I would like to do it");
        assert!(!title.is_empty());
    }

    #[test]
    fn test_keywords_sentence_boundary() {
        let title = extract_title_keywords(
            "Fix the login bug. Also update the dashboard and refactor the API layer.",
        );
        // Should only use first sentence
        assert!(
            !title.contains("Dashboard") && !title.contains("Api"),
            "should stop at first sentence: {title}"
        );
    }

    #[test]
    fn test_keywords_multibyte_no_panic() {
        let content = format!("{}. Second sentence.", "日本語テスト".repeat(50));
        let title = extract_title_keywords(&content);
        assert!(!title.is_empty());
    }

    #[test]
    fn test_char_limit_byte_index() {
        let s = "héllo wörld";
        let idx = char_limit_byte_index(s, 3);
        assert!(s.is_char_boundary(idx));
        assert_eq!(&s[..idx].chars().count(), &3);
    }

    #[test]
    fn test_truncate_cjk_80_chars() {
        // 100 CJK characters — should truncate to ~80 chars, not ~26 (which is 80 bytes)
        let content = "界".repeat(100);
        let title = truncate_to_title(&content);
        assert!(title.ends_with('…'));
        // Should be 80 CJK chars + ellipsis = 81 chars
        assert_eq!(title.chars().count(), 81);
    }
}
