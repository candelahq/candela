//! Response body parsing and token usage extraction.
//!
//! Ported from: `pkg/proxy/parsers.go`
//!
//! TODO: Implement parsers for OpenAI, Anthropic, and Gemini response formats.

/// Extracted token usage from an LLM response.
#[derive(Debug, Default)]
pub struct TokenUsage {
    pub input_tokens: i64,
    pub output_tokens: i64,
    pub total_tokens: i64,
}
