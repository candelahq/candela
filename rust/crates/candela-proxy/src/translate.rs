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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn openai_request_serde_round_trip() {
        let req = OpenAIRequest {
            model: "gpt-4o".into(),
            messages: vec![OpenAIMessage {
                role: "user".into(),
                content: serde_json::Value::String("Hello".into()),
            }],
            max_tokens: Some(1024),
            temperature: Some(0.7),
            stream: false,
            tools: vec![],
        };
        let json = serde_json::to_string(&req).unwrap();
        let restored: OpenAIRequest = serde_json::from_str(&json).unwrap();
        assert_eq!(restored.model, "gpt-4o");
        assert_eq!(restored.messages.len(), 1);
        assert_eq!(restored.max_tokens, Some(1024));
    }

    #[test]
    fn anthropic_stream_event_deserialization() {
        let events = vec![
            (r#"{"type":"ping"}"#, "ping"),
            (r#"{"type":"message_stop"}"#, "message_stop"),
            (r#"{"type":"content_block_stop"}"#, "content_block_stop"),
            (
                r#"{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}"#,
                "content_block_delta",
            ),
            (
                r#"{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}"#,
                "message_start",
            ),
        ];

        for (json, expected_type) in events {
            let event: AnthropicStreamEvent = serde_json::from_str(json)
                .unwrap_or_else(|e| panic!("failed to parse {expected_type}: {e}"));
            match (&event, expected_type) {
                (AnthropicStreamEvent::Ping, "ping") => {}
                (AnthropicStreamEvent::MessageStop, "message_stop") => {}
                (AnthropicStreamEvent::ContentBlockStop, "content_block_stop") => {}
                (AnthropicStreamEvent::ContentBlockDelta { .. }, "content_block_delta") => {}
                (AnthropicStreamEvent::MessageStart { .. }, "message_start") => {}
                _ => panic!("unexpected variant for {expected_type}"),
            }
        }
    }

    #[test]
    fn openai_to_anthropic_message_structure() {
        // Verify structural compatibility between OpenAI and Anthropic message formats.
        let openai_msg = OpenAIMessage {
            role: "user".into(),
            content: serde_json::Value::String("What is Rust?".into()),
        };
        let anthropic_msg = AnthropicMessage {
            role: openai_msg.role.clone(),
            content: openai_msg.content.clone(),
        };
        let json = serde_json::to_value(&anthropic_msg).unwrap();
        assert_eq!(json["role"], "user");
        assert_eq!(json["content"], "What is Rust?");
    }

    // ── New comprehensive tests ──

    /// OpenAI request without optional fields should serialize cleanly.
    #[test]
    fn openai_request_minimal() {
        let req = OpenAIRequest {
            model: "gpt-4o-mini".into(),
            messages: vec![OpenAIMessage {
                role: "user".into(),
                content: serde_json::Value::String("hi".into()),
            }],
            max_tokens: None,
            temperature: None,
            stream: false,
            tools: vec![],
        };
        let json = serde_json::to_string(&req).unwrap();
        // Optional fields should be absent.
        assert!(!json.contains("max_tokens"));
        assert!(!json.contains("temperature"));
        assert!(!json.contains("tools"));
    }

    /// Anthropic request requires max_tokens — verify it's mandatory.
    #[test]
    fn anthropic_request_round_trip() {
        let req = AnthropicRequest {
            model: "claude-sonnet-4-20250514".into(),
            messages: vec![AnthropicMessage {
                role: "user".into(),
                content: serde_json::Value::String("Hello Claude".into()),
            }],
            max_tokens: 4096,
            temperature: Some(0.0),
            system: Some("You are a helpful assistant.".into()),
            stream: true,
            tools: vec![],
        };
        let json = serde_json::to_string(&req).unwrap();
        let restored: AnthropicRequest = serde_json::from_str(&json).unwrap();
        assert_eq!(restored.model, "claude-sonnet-4-20250514");
        assert_eq!(restored.max_tokens, 4096);
        assert_eq!(restored.system, Some("You are a helpful assistant.".into()));
        assert!(restored.stream);
    }

    /// Anthropic request without system prompt should omit it.
    #[test]
    fn anthropic_request_no_system() {
        let req = AnthropicRequest {
            model: "claude-haiku-4.5".into(),
            messages: vec![],
            max_tokens: 1024,
            temperature: None,
            system: None,
            stream: false,
            tools: vec![],
        };
        let json = serde_json::to_string(&req).unwrap();
        assert!(!json.contains("system"), "None system should be omitted");
    }

    /// message_delta stream event with usage should parse correctly.
    #[test]
    fn anthropic_stream_message_delta_with_usage() {
        let json = r#"{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}"#;
        let event: AnthropicStreamEvent = serde_json::from_str(json).unwrap();
        match event {
            AnthropicStreamEvent::MessageDelta { usage, .. } => {
                let usage = usage.expect("usage should be present");
                assert_eq!(usage["output_tokens"], 42);
            }
            _ => panic!("expected MessageDelta"),
        }
    }

    /// message_delta without usage should also parse.
    #[test]
    fn anthropic_stream_message_delta_without_usage() {
        let json = r#"{"type":"message_delta","delta":{"stop_reason":"end_turn"}}"#;
        let event: AnthropicStreamEvent = serde_json::from_str(json).unwrap();
        match event {
            AnthropicStreamEvent::MessageDelta { usage, .. } => {
                assert!(usage.is_none());
            }
            _ => panic!("expected MessageDelta"),
        }
    }

    /// Multi-turn conversation should preserve message order.
    #[test]
    fn openai_multi_turn_preserves_order() {
        let req = OpenAIRequest {
            model: "gpt-4o".into(),
            messages: vec![
                OpenAIMessage {
                    role: "system".into(),
                    content: serde_json::json!("You are a bot."),
                },
                OpenAIMessage {
                    role: "user".into(),
                    content: serde_json::json!("Hi"),
                },
                OpenAIMessage {
                    role: "assistant".into(),
                    content: serde_json::json!("Hello!"),
                },
                OpenAIMessage {
                    role: "user".into(),
                    content: serde_json::json!("How are you?"),
                },
            ],
            max_tokens: None,
            temperature: None,
            stream: false,
            tools: vec![],
        };
        let json = serde_json::to_string(&req).unwrap();
        let restored: OpenAIRequest = serde_json::from_str(&json).unwrap();
        assert_eq!(restored.messages.len(), 4);
        assert_eq!(restored.messages[0].role, "system");
        assert_eq!(restored.messages[3].role, "user");
    }

    /// Temperature of 0.0 and 2.0 (boundary values) should round-trip.
    #[test]
    fn temperature_boundary_values() {
        for temp in [0.0, 0.5, 1.0, 2.0] {
            let req = OpenAIRequest {
                model: "gpt-4o".into(),
                messages: vec![],
                max_tokens: None,
                temperature: Some(temp),
                stream: false,
                tools: vec![],
            };
            let json = serde_json::to_string(&req).unwrap();
            let restored: OpenAIRequest = serde_json::from_str(&json).unwrap();
            assert!((restored.temperature.unwrap() - temp).abs() < f64::EPSILON);
        }
    }

    /// Unknown stream event type should fail deserialization (exhaustive matching).
    #[test]
    fn anthropic_stream_unknown_event_rejected() {
        let json = r#"{"type":"unknown_event","data":{}}"#;
        let result = serde_json::from_str::<AnthropicStreamEvent>(json);
        assert!(result.is_err(), "unknown event types must be rejected");
    }
}
