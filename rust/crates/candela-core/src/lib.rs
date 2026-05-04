//! Core domain types and trait definitions for the Candela LLM observability platform.
//!
//! This crate defines the foundational types that every other crate depends on:
//! [`Span`], [`GenAIAttributes`], [`SpanKind`], [`SpanStatus`], and the
//! [`SpanWriter`] trait for storage backends.
//!
//! Ported from: `pkg/storage/store.go`

use std::collections::BTreeMap;
use std::time::Duration;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

pub mod proto;

// ── Error Types ──

/// Standard error type used across Candela crates.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("not found")]
    NotFound,

    #[error("{0}")]
    Internal(#[from] anyhow::Error),
}

// ── Domain Enums ──

/// Span kind — mirrors the proto `SpanKind` enum.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "snake_case")]
#[repr(i32)]
pub enum SpanKind {
    #[default]
    Unspecified = 0,
    Llm = 1,
    Agent = 2,
    Tool = 3,
    Retrieval = 4,
    Embedding = 5,
    Chain = 6,
    General = 7,
}

/// Span status — mirrors the proto `SpanStatus` enum.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "snake_case")]
#[repr(i32)]
pub enum SpanStatus {
    #[default]
    Unspecified = 0,
    Ok = 1,
    Error = 2,
}

// ── Domain Structs ──

/// LLM-specific attributes attached to a span.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct GenAIAttributes {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub model: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub provider: String,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub input_tokens: i64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub output_tokens: i64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub total_tokens: i64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub cost_usd: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub temperature: f64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub max_tokens: i64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub top_p: f64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub input_content: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub output_content: String,
}

/// A single observability span in the storage layer.
///
/// Represents a captured LLM API call, agent step, tool invocation,
/// or other instrumented operation.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Span {
    pub span_id: String,
    pub trace_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parent_span_id: Option<String>,
    pub name: String,
    #[serde(default)]
    pub kind: SpanKind,
    #[serde(default)]
    pub status: SpanStatus,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status_message: Option<String>,
    pub start_time: DateTime<Utc>,
    pub end_time: DateTime<Utc>,
    #[serde(with = "duration_serde")]
    pub duration: Duration,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub gen_ai: Option<GenAIAttributes>,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub attributes: BTreeMap<String, String>,
    pub project_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub environment: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub service_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_id: Option<String>,
}

// ── SpanWriter Trait ──

/// Async trait for span ingestion sinks (Pub/Sub, OTLP, DuckDB, etc.).
///
/// Implementations receive batches of spans and write them to their
/// respective backends. Errors are logged but do not halt the pipeline.
pub trait SpanWriter: Send + Sync {
    /// Ingest a batch of spans into the storage backend.
    fn ingest_spans(
        &self,
        spans: &[Span],
    ) -> impl std::future::Future<Output = anyhow::Result<()>> + Send;

    /// Flush any pending writes and release resources.
    fn close(&self) -> impl std::future::Future<Output = anyhow::Result<()>> + Send {
        async { Ok(()) }
    }
}

// ── Helper Functions ──

fn is_zero_i64(v: &i64) -> bool {
    *v == 0
}

fn is_zero_f64(v: &f64) -> bool {
    *v == 0.0
}

/// Serde module for `std::time::Duration` as floating-point seconds.
mod duration_serde {
    use serde::{self, Deserialize, Deserializer, Serializer};
    use std::time::Duration;

    pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_f64(duration.as_secs_f64())
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
    where
        D: Deserializer<'de>,
    {
        let secs = f64::deserialize(deserializer)?;
        Ok(Duration::from_secs_f64(secs))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn span_json_round_trip() {
        let span = Span {
            span_id: "abc123".into(),
            trace_id: "trace456".into(),
            parent_span_id: None,
            name: "llm.chat".into(),
            kind: SpanKind::Llm,
            status: SpanStatus::Ok,
            status_message: None,
            start_time: Utc::now(),
            end_time: Utc::now(),
            duration: Duration::from_millis(250),
            gen_ai: Some(GenAIAttributes {
                model: "gpt-4".into(),
                provider: "openai".into(),
                input_tokens: 100,
                output_tokens: 50,
                total_tokens: 150,
                cost_usd: 0.003,
                ..Default::default()
            }),
            attributes: BTreeMap::new(),
            project_id: "test-project".into(),
            environment: Some("dev".into()),
            service_name: None,
            user_id: None,
            session_id: None,
        };

        let json = serde_json::to_string(&span).expect("serialize");
        let restored: Span = serde_json::from_str(&json).expect("deserialize");

        assert_eq!(restored.span_id, span.span_id);
        assert_eq!(restored.kind, SpanKind::Llm);
        assert_eq!(restored.gen_ai.as_ref().unwrap().model, "gpt-4");
        assert_eq!(restored.gen_ai.as_ref().unwrap().input_tokens, 100);
    }
}
