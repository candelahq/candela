//! Chat runtime — manages the conversation loop, model calls, and streaming.
//!
//! This crate handles:
//! - Assembling context (system prompt + history + pinned context)
//! - Calling LLM providers via Candela proxy or direct API
//! - Streaming responses back to the IDE
//! - Tool call execution and approval flow
//! - Budget enforcement

pub mod client;
pub mod runtime;

pub use client::ModelClient;
pub use runtime::ChatRuntime;
