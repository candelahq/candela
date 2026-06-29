//! Chat runtime — manages the conversation loop, model calls, and streaming.
//!
//! This crate will handle:
//! - Assembling context (system prompt + history + pinned context)
//! - Calling LLM providers via Candela proxy
//! - Streaming responses back to the IDE
//! - Tool call execution and approval flow
//! - Budget enforcement

pub mod runtime;

pub use runtime::ChatRuntime;
