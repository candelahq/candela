//! Response body parsing and token usage extraction.
//!
//! Ported from: `pkg/proxy/parsers.go`
//!
//! Provides provider-aware parsing for OpenAI, Anthropic, Google, and a
//! fallback parser for unknown providers. Each parser extracts token usage
//! from both standard JSON responses and SSE streaming data.

use serde_json::Value;

/// Extracted token usage from an LLM response.
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct TokenUsage {
    pub input_tokens: i64,
    pub output_tokens: i64,
    pub total_tokens: i64,
}

/// Cache token counts extracted from an LLM response.
/// All three providers report caching differently:
/// - **Anthropic**: `cache_read_input_tokens` + `cache_creation_input_tokens`
/// - **OpenAI**: `usage.prompt_tokens_details.cached_tokens`
/// - **Google**: `usageMetadata.cachedContentTokenCount`
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct CacheTokens {
    pub cache_read_tokens: i64,
    pub cache_creation_tokens: i64,
}

// ── Provider Parser Trait ──

/// Provider-specific response parser.
///
/// Mirrors Go's `ProviderParser` interface in `parsers.go`.
pub trait ProviderParser: Send + Sync {
    /// Returns true if the request body indicates streaming.
    fn is_streaming(&self, body: &[u8]) -> bool;

    /// Extract model name and input content from a request body.
    fn parse_request(&self, body: &[u8]) -> (String, String);

    /// Extract output content and token usage from a standard response.
    fn parse_response(&self, body: &[u8]) -> (String, TokenUsage);

    /// Extract output content and token usage from accumulated SSE stream data.
    fn parse_streaming_response(&self, data: &[u8]) -> (String, TokenUsage);
}

/// Returns the appropriate parser for a provider name.
pub fn get_parser(provider: &str) -> Box<dyn ProviderParser> {
    match provider {
        "openai" | "gemini-oai" => Box::new(OpenAIParser),
        "anthropic" | "anthropic-direct" | "anthropic-vertex" => Box::new(AnthropicParser),
        "google" => Box::new(GoogleParser),
        _ => Box::new(FallbackParser),
    }
}

// ── Cache Token Extraction (All Providers) ──

/// Extract cache token counts from a standard response body.
pub fn extract_cache_tokens(provider: &str, body: &[u8]) -> CacheTokens {
    let val: Value = match serde_json::from_slice(body) {
        Ok(v) => v,
        Err(_) => return CacheTokens::default(),
    };

    match provider {
        "anthropic" | "anthropic-direct" | "anthropic-vertex" => {
            if let Some(usage) = val.get("usage").and_then(|u| u.as_object()) {
                CacheTokens {
                    cache_read_tokens: usage
                        .get("cache_read_input_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0),
                    cache_creation_tokens: usage
                        .get("cache_creation_input_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0),
                }
            } else {
                CacheTokens::default()
            }
        }
        "openai" | "gemini-oai" => {
            if let Some(details) = val
                .get("usage")
                .and_then(|u| u.get("prompt_tokens_details"))
                .and_then(|d| d.as_object())
            {
                CacheTokens {
                    cache_read_tokens: details
                        .get("cached_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0),
                    cache_creation_tokens: 0,
                }
            } else {
                CacheTokens::default()
            }
        }
        "google" => {
            if let Some(meta) = val.get("usageMetadata").and_then(|m| m.as_object()) {
                CacheTokens {
                    cache_read_tokens: meta
                        .get("cachedContentTokenCount")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0),
                    cache_creation_tokens: 0,
                }
            } else {
                CacheTokens::default()
            }
        }
        _ => CacheTokens::default(),
    }
}

/// Extract cache token counts from accumulated SSE streaming data.
pub fn extract_streaming_cache_tokens(provider: &str, data: &[u8]) -> CacheTokens {
    let text = String::from_utf8_lossy(data);

    match provider {
        "anthropic" | "anthropic-direct" | "anthropic-vertex" => {
            // Anthropic puts cache tokens in `message_start` event's usage block.
            for line in text.lines() {
                let line = line.trim();
                if !line.starts_with("data: ") {
                    continue;
                }
                let payload = &line["data: ".len()..];
                if payload == "[DONE]" {
                    continue;
                }
                let chunk: Value = match serde_json::from_str(payload) {
                    Ok(v) => v,
                    Err(_) => continue,
                };
                // Check message_start event → message.usage
                if let Some(usage) = chunk
                    .get("message")
                    .and_then(|m| m.get("usage"))
                    .and_then(|u| u.as_object())
                {
                    let read = usage
                        .get("cache_read_input_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0);
                    let creation = usage
                        .get("cache_creation_input_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0);
                    if read > 0 || creation > 0 {
                        return CacheTokens {
                            cache_read_tokens: read,
                            cache_creation_tokens: creation,
                        };
                    }
                }
            }
            CacheTokens::default()
        }
        // OpenAI streaming: usage in the final SSE chunk.
        "openai" | "gemini-oai" => {
            for line in text.lines() {
                let line = line.trim();
                if !line.starts_with("data: ") {
                    continue;
                }
                let payload = &line["data: ".len()..];
                if payload == "[DONE]" {
                    continue;
                }
                let chunk: Value = match serde_json::from_str(payload) {
                    Ok(v) => v,
                    Err(_) => continue,
                };
                if let Some(details) = chunk
                    .get("usage")
                    .and_then(|u| u.get("prompt_tokens_details"))
                    .and_then(|d| d.as_object())
                {
                    let read = details
                        .get("cached_tokens")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0);
                    if read > 0 {
                        return CacheTokens {
                            cache_read_tokens: read,
                            cache_creation_tokens: 0,
                        };
                    }
                }
            }
            CacheTokens::default()
        }
        // Google streaming: JSON array or newline-delimited chunks.
        "google" => {
            for line in text.lines() {
                let line = line.trim();
                // Google streams can be JSON arrays or newline-delimited objects.
                let chunk: Value = match serde_json::from_str(line) {
                    Ok(v) => v,
                    Err(_) => continue,
                };
                if let Some(meta) = chunk.get("usageMetadata").and_then(|m| m.as_object()) {
                    let read = meta
                        .get("cachedContentTokenCount")
                        .and_then(|v| v.as_i64())
                        .unwrap_or(0);
                    if read > 0 {
                        return CacheTokens {
                            cache_read_tokens: read,
                            cache_creation_tokens: 0,
                        };
                    }
                }
            }
            CacheTokens::default()
        }
        _ => CacheTokens::default(),
    }
}

// ── OpenAI Parser ──

struct OpenAIParser;

impl ProviderParser for OpenAIParser {
    fn is_streaming(&self, body: &[u8]) -> bool {
        serde_json::from_slice::<Value>(body)
            .ok()
            .and_then(|v| v.get("stream")?.as_bool())
            .unwrap_or(false)
    }

    fn parse_request(&self, body: &[u8]) -> (String, String) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), String::new()),
        };
        let model = val
            .get("model")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let content = val
            .get("messages")
            .map(|m| m.to_string())
            .unwrap_or_default();
        (model, content)
    }

    fn parse_response(&self, body: &[u8]) -> (String, TokenUsage) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), TokenUsage::default()),
        };

        let usage = if let Some(u) = val.get("usage").and_then(|u| u.as_object()) {
            let input = u.get("prompt_tokens").and_then(|v| v.as_i64()).unwrap_or(0);
            let output = u
                .get("completion_tokens")
                .and_then(|v| v.as_i64())
                .unwrap_or(0);
            let total = u
                .get("total_tokens")
                .and_then(|v| v.as_i64())
                .unwrap_or_else(|| input.saturating_add(output));
            TokenUsage {
                input_tokens: input,
                output_tokens: output,
                total_tokens: total,
            }
        } else {
            TokenUsage::default()
        };

        let content = val
            .get("choices")
            .and_then(|c| c.as_array())
            .and_then(|arr| arr.first())
            .and_then(|c| c.get("message"))
            .and_then(|m| m.get("content"))
            .and_then(|c| c.as_str())
            .unwrap_or("")
            .to_string();

        (content, usage)
    }

    fn parse_streaming_response(&self, data: &[u8]) -> (String, TokenUsage) {
        let text = String::from_utf8_lossy(data);
        let mut content = String::new();
        let mut usage = TokenUsage::default();

        for line in text.lines() {
            let line = line.trim();
            if !line.starts_with("data: ") {
                continue;
            }
            let payload = &line["data: ".len()..];
            if payload == "[DONE]" {
                continue;
            }
            let chunk: Value = match serde_json::from_str(payload) {
                Ok(v) => v,
                Err(_) => continue,
            };

            // Content deltas
            if let Some(delta_content) = chunk
                .get("choices")
                .and_then(|c| c.as_array())
                .and_then(|arr| arr.first())
                .and_then(|c| c.get("delta"))
                .and_then(|d| d.get("content"))
                .and_then(|c| c.as_str())
            {
                content.push_str(delta_content);
            }

            // Usage (appears in final chunk)
            if let Some(u) = chunk.get("usage").and_then(|u| u.as_object()) {
                usage.input_tokens = u.get("prompt_tokens").and_then(|v| v.as_i64()).unwrap_or(0);
                usage.output_tokens = u
                    .get("completion_tokens")
                    .and_then(|v| v.as_i64())
                    .unwrap_or(0);
                usage.total_tokens = u
                    .get("total_tokens")
                    .and_then(|v| v.as_i64())
                    .unwrap_or_else(|| usage.input_tokens.saturating_add(usage.output_tokens));
            }
        }

        (content, usage)
    }
}

// ── Anthropic Parser ──

struct AnthropicParser;

impl ProviderParser for AnthropicParser {
    fn is_streaming(&self, body: &[u8]) -> bool {
        serde_json::from_slice::<Value>(body)
            .ok()
            .and_then(|v| v.get("stream")?.as_bool())
            .unwrap_or(false)
    }

    fn parse_request(&self, body: &[u8]) -> (String, String) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), String::new()),
        };
        let model = val
            .get("model")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let content = val
            .get("messages")
            .map(|m| m.to_string())
            .unwrap_or_default();
        (model, content)
    }

    fn parse_response(&self, body: &[u8]) -> (String, TokenUsage) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), TokenUsage::default()),
        };

        let usage = if let Some(u) = val.get("usage").and_then(|u| u.as_object()) {
            let input = u.get("input_tokens").and_then(|v| v.as_i64()).unwrap_or(0);
            let output = u.get("output_tokens").and_then(|v| v.as_i64()).unwrap_or(0);
            TokenUsage {
                input_tokens: input,
                output_tokens: output,
                total_tokens: input.saturating_add(output),
            }
        } else {
            TokenUsage::default()
        };

        let content = val
            .get("content")
            .and_then(|c| c.as_array())
            .and_then(|arr| arr.first())
            .and_then(|b| b.get("text"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();

        (content, usage)
    }

    #[allow(clippy::collapsible_if)] // Intentional: avoid unstable let_chains for stable Rust.
    fn parse_streaming_response(&self, data: &[u8]) -> (String, TokenUsage) {
        let text = String::from_utf8_lossy(data);
        let mut content = String::new();
        let mut input_tokens: i64 = 0;
        let mut output_tokens: i64 = 0;

        for line in text.lines() {
            let line = line.trim();
            if !line.starts_with("data: ") {
                continue;
            }
            let payload = &line["data: ".len()..];
            if payload == "[DONE]" {
                continue;
            }
            let chunk: Value = match serde_json::from_str(payload) {
                Ok(v) => v,
                Err(_) => continue,
            };

            // Content deltas (content_block_delta events)
            if let Some(text_delta) = chunk
                .get("delta")
                .and_then(|d| d.get("text"))
                .and_then(|t| t.as_str())
            {
                content.push_str(text_delta);
            }

            // Output tokens from message_delta usage
            if let Some(u) = chunk.get("usage").and_then(|u| u.as_object()) {
                if let Some(v) = u.get("output_tokens").and_then(|v| v.as_i64()) {
                    if v > 0 {
                        output_tokens = v;
                    }
                }
            }

            // Input tokens from message_start → message.usage
            if let Some(u) = chunk
                .get("message")
                .and_then(|m| m.get("usage"))
                .and_then(|u| u.as_object())
            {
                if let Some(v) = u.get("input_tokens").and_then(|v| v.as_i64()) {
                    input_tokens = v;
                }
            }
        }

        let usage = TokenUsage {
            input_tokens,
            output_tokens,
            total_tokens: input_tokens.saturating_add(output_tokens),
        };

        (content, usage)
    }
}

// ── Google Parser ──

struct GoogleParser;

impl ProviderParser for GoogleParser {
    fn is_streaming(&self, _body: &[u8]) -> bool {
        // Google uses a different endpoint for streaming, not a body param.
        false
    }

    fn parse_request(&self, body: &[u8]) -> (String, String) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), String::new()),
        };
        // Model is in the URL path for Google, not the body.
        let content = val
            .get("contents")
            .map(|c| c.to_string())
            .unwrap_or_default();
        (String::new(), content)
    }

    fn parse_response(&self, body: &[u8]) -> (String, TokenUsage) {
        let val: Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => return (String::new(), TokenUsage::default()),
        };

        let usage = if let Some(meta) = val.get("usageMetadata").and_then(|m| m.as_object()) {
            let input = meta
                .get("promptTokenCount")
                .and_then(|v| v.as_i64())
                .unwrap_or(0);
            let output = meta
                .get("candidatesTokenCount")
                .and_then(|v| v.as_i64())
                .unwrap_or(0);
            TokenUsage {
                input_tokens: input,
                output_tokens: output,
                total_tokens: input.saturating_add(output),
            }
        } else {
            TokenUsage::default()
        };

        let content = val
            .get("candidates")
            .and_then(|c| c.as_array())
            .and_then(|arr| arr.first())
            .and_then(|c| c.get("content"))
            .and_then(|c| c.get("parts"))
            .and_then(|p| p.as_array())
            .and_then(|arr| arr.first())
            .and_then(|p| p.get("text"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();

        (content, usage)
    }

    fn parse_streaming_response(&self, data: &[u8]) -> (String, TokenUsage) {
        // Google streaming uses a different endpoint — fall back to standard parsing.
        self.parse_response(data)
    }
}

// ── Fallback Parser ──

struct FallbackParser;

impl ProviderParser for FallbackParser {
    fn is_streaming(&self, _body: &[u8]) -> bool {
        false
    }
    fn parse_request(&self, _body: &[u8]) -> (String, String) {
        (String::new(), String::new())
    }
    fn parse_response(&self, _body: &[u8]) -> (String, TokenUsage) {
        (String::new(), TokenUsage::default())
    }
    fn parse_streaming_response(&self, _data: &[u8]) -> (String, TokenUsage) {
        (String::new(), TokenUsage::default())
    }
}

// ── Helpers ──

/// Provider-aware token extraction from a standard JSON response body.
/// Replaces the old provider-agnostic `extract_token_usage` in handler.rs.
pub fn extract_response_usage(provider: &str, body: &[u8]) -> (String, TokenUsage) {
    get_parser(provider).parse_response(body)
}

/// Provider-aware token extraction from accumulated SSE stream data.
pub fn extract_streaming_usage(provider: &str, data: &[u8]) -> (String, TokenUsage) {
    get_parser(provider).parse_streaming_response(data)
}

/// Extract model name and input content from a request body.
pub fn extract_request_info(provider: &str, body: &[u8]) -> (String, String) {
    get_parser(provider).parse_request(body)
}

/// Provider-aware streaming detection.
pub fn is_streaming_request(provider: &str, body: &[u8]) -> bool {
    get_parser(provider).is_streaming(body)
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── OpenAI standard ──

    #[test]
    fn openai_parse_response() {
        let body = br#"{"choices":[{"message":{"content":"Hello!"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}"#;
        let (content, usage) = extract_response_usage("openai", body);
        assert_eq!(content, "Hello!");
        assert_eq!(usage.input_tokens, 100);
        assert_eq!(usage.output_tokens, 50);
        assert_eq!(usage.total_tokens, 150);
    }

    #[test]
    fn openai_parse_streaming() {
        let data = b"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\ndata: {\"choices\":[{\"delta\":{\"content\":\" there\"}}]}\ndata: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\ndata: [DONE]\n";
        let (content, usage) = extract_streaming_usage("openai", data);
        assert_eq!(content, "Hi there");
        assert_eq!(usage.input_tokens, 10);
        assert_eq!(usage.output_tokens, 5);
        assert_eq!(usage.total_tokens, 15);
    }

    #[test]
    fn gemini_oai_uses_openai_parser() {
        let body = br#"{"usage":{"prompt_tokens":42,"completion_tokens":8,"total_tokens":50}}"#;
        let (_, usage) = extract_response_usage("gemini-oai", body);
        assert_eq!(usage.input_tokens, 42);
        assert_eq!(usage.output_tokens, 8);
    }

    // ── Anthropic standard ──

    #[test]
    fn anthropic_parse_response() {
        let body = br#"{"content":[{"type":"text","text":"Hello from Claude!"}],"usage":{"input_tokens":200,"output_tokens":80}}"#;
        let (content, usage) = extract_response_usage("anthropic", body);
        assert_eq!(content, "Hello from Claude!");
        assert_eq!(usage.input_tokens, 200);
        assert_eq!(usage.output_tokens, 80);
        assert_eq!(usage.total_tokens, 280);
    }

    #[test]
    fn anthropic_parse_streaming() {
        let data = b"event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":100}}}\nevent: content_block_delta\ndata: {\"delta\":{\"text\":\"Hey\"}}\nevent: message_delta\ndata: {\"usage\":{\"output_tokens\":25}}\ndata: [DONE]\n";
        let (content, usage) = extract_streaming_usage("anthropic", data);
        assert_eq!(content, "Hey");
        assert_eq!(usage.input_tokens, 100);
        assert_eq!(usage.output_tokens, 25);
        assert_eq!(usage.total_tokens, 125);
    }

    #[test]
    fn anthropic_auto_computes_total() {
        let body = br#"{"usage":{"input_tokens":300,"output_tokens":120}}"#;
        let (_, usage) = extract_response_usage("anthropic", body);
        assert_eq!(usage.total_tokens, 420);
    }

    // ── Google standard ──

    #[test]
    fn google_parse_response() {
        let body = br#"{"candidates":[{"content":{"parts":[{"text":"Hi from Gemini"}]}}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":20}}"#;
        let (content, usage) = extract_response_usage("google", body);
        assert_eq!(content, "Hi from Gemini");
        assert_eq!(usage.input_tokens, 50);
        assert_eq!(usage.output_tokens, 20);
        assert_eq!(usage.total_tokens, 70);
    }

    // ── Cache token extraction ──

    #[test]
    fn cache_tokens_anthropic() {
        let body = br#"{"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":80,"cache_creation_input_tokens":20}}"#;
        let cache = extract_cache_tokens("anthropic", body);
        assert_eq!(cache.cache_read_tokens, 80);
        assert_eq!(cache.cache_creation_tokens, 20);
    }

    #[test]
    fn cache_tokens_anthropic_aliases() {
        let body = br#"{"usage":{"cache_read_input_tokens":42}}"#;
        for provider in &["anthropic", "anthropic-direct", "anthropic-vertex"] {
            let cache = extract_cache_tokens(provider, body);
            assert_eq!(cache.cache_read_tokens, 42, "failed for {provider}");
        }
    }

    #[test]
    fn cache_tokens_openai() {
        let body =
            br#"{"usage":{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":60}}}"#;
        let cache = extract_cache_tokens("openai", body);
        assert_eq!(cache.cache_read_tokens, 60);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    #[test]
    fn cache_tokens_google() {
        let body = br#"{"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":45}}"#;
        let cache = extract_cache_tokens("google", body);
        assert_eq!(cache.cache_read_tokens, 45);
    }

    #[test]
    fn cache_tokens_unknown_provider() {
        let body = br#"{"usage":{"cache_read_input_tokens":99}}"#;
        let cache = extract_cache_tokens("unknown", body);
        assert_eq!(cache.cache_read_tokens, 0);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    #[test]
    fn streaming_cache_tokens_anthropic() {
        let data = b"event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":70,\"cache_creation_input_tokens\":10}}}\n";
        let cache = extract_streaming_cache_tokens("anthropic", data);
        assert_eq!(cache.cache_read_tokens, 70);
        assert_eq!(cache.cache_creation_tokens, 10);
    }

    #[test]
    fn streaming_cache_tokens_openai() {
        let data = b"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\ndata: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":60}}}\ndata: [DONE]\n";
        let cache = extract_streaming_cache_tokens("openai", data);
        assert_eq!(cache.cache_read_tokens, 60);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    #[test]
    fn streaming_cache_tokens_gemini_oai() {
        let data = b"data: {\"usage\":{\"prompt_tokens\":200,\"prompt_tokens_details\":{\"cached_tokens\":120}}}\ndata: [DONE]\n";
        let cache = extract_streaming_cache_tokens("gemini-oai", data);
        assert_eq!(cache.cache_read_tokens, 120);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    #[test]
    fn streaming_cache_tokens_google() {
        let data =
            b"{\"usageMetadata\":{\"promptTokenCount\":100,\"cachedContentTokenCount\":45}}\n";
        let cache = extract_streaming_cache_tokens("google", data);
        assert_eq!(cache.cache_read_tokens, 45);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    #[test]
    fn streaming_cache_tokens_openai_no_cache() {
        // Streaming response with no cache data should return defaults.
        let data = b"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\ndata: {\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":10}}\ndata: [DONE]\n";
        let cache = extract_streaming_cache_tokens("openai", data);
        assert_eq!(cache.cache_read_tokens, 0);
        assert_eq!(cache.cache_creation_tokens, 0);
    }

    // ── Fallback / edge cases ──

    #[test]
    fn unknown_provider_returns_fallback() {
        let body = br#"{"usage":{"prompt_tokens":100}}"#;
        let (_, usage) = extract_response_usage("unknown-provider", body);
        assert_eq!(usage.input_tokens, 0);
    }

    #[test]
    fn empty_body_no_panic() {
        let (_, usage) = extract_response_usage("openai", b"");
        assert_eq!(usage, TokenUsage::default());
    }

    #[test]
    fn invalid_json_no_panic() {
        let (_, usage) = extract_response_usage("anthropic", b"not json at all");
        assert_eq!(usage, TokenUsage::default());
    }

    #[test]
    fn extract_request_info_openai() {
        let body = br#"{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}"#;
        let (model, content) = extract_request_info("openai", body);
        assert_eq!(model, "gpt-4o");
        assert!(content.contains("user"));
    }

    #[test]
    fn is_streaming_detection() {
        let body = br#"{"model":"gpt-4","stream":true}"#;
        assert!(is_streaming_request("openai", body));
        assert!(is_streaming_request("anthropic", body));

        let body = br#"{"model":"gpt-4","stream":false}"#;
        assert!(!is_streaming_request("openai", body));

        // Google uses different endpoints, not body param
        assert!(!is_streaming_request("google", br#"{"stream":true}"#));
    }
}
