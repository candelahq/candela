//! Storage backend implementations for Candela span export.
//!
//! Provides [`SpanWriter`] implementations for:
//! - Google Cloud Pub/Sub (proto and JSON formats)
//! - OTLP/HTTP (OpenTelemetry collector export)

pub mod otlp;
pub mod pubsub;
