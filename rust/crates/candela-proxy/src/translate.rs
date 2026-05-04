//! Format translation between LLM API formats (OpenAI ↔ Anthropic).
//!
//! Ported from: `pkg/proxy/translate.go`
//!
//! Key improvement over Go: fully typed request/response structs via serde,
//! replacing `map[string]interface{}` with exhaustive enum matching.
//!
//! TODO: Implement full translation logic (#123).

use serde::{Deserialize, Serialize};

// ── OpenAI Types ──

#[derive(Debug, Serialize, Deserialize)]
pub struct OpenAIRequest {
    pub model: String,
    pub messages: Vec<OpenAIMessage>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    #[serde(default)]
    pub stream: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tools: Vec<serde_json::Value>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct OpenAIMessage {
    pub role: String,
    pub content: serde_json::Value,
}

// ── Anthropic Types ──

#[derive(Debug, Serialize, Deserialize)]
pub struct AnthropicRequest {
    pub model: String,
    pub messages: Vec<AnthropicMessage>,
    pub max_tokens: u32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system: Option<String>,
    #[serde(default)]
    pub stream: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tools: Vec<serde_json::Value>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AnthropicMessage {
    pub role: String,
    pub content: serde_json::Value,
}

/// Anthropic SSE stream events — exhaustive matching, no silent failures.
#[derive(Debug, Deserialize)]
#[serde(tag = "type")]
pub enum AnthropicStreamEvent {
    #[serde(rename = "message_start")]
    MessageStart { message: serde_json::Value },
    #[serde(rename = "content_block_start")]
    ContentBlockStart { content_block: serde_json::Value },
    #[serde(rename = "content_block_delta")]
    ContentBlockDelta { delta: serde_json::Value },
    #[serde(rename = "content_block_stop")]
    ContentBlockStop,
    #[serde(rename = "message_delta")]
    MessageDelta {
        delta: serde_json::Value,
        usage: Option<serde_json::Value>,
    },
    #[serde(rename = "message_stop")]
    MessageStop,
    #[serde(rename = "ping")]
    Ping,
}
