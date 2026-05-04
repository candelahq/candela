//! Re-exports for buf-generated prost types.
//!
//! After running `buf generate`, the generated Rust types land in `gen/src/`.
//! This module will re-export them for use across the workspace.
//!
//! TODO: Wire up after `buf generate` is configured (#120).

// Example (uncomment after codegen):
// include!("../../../gen/src/candela.types.rs");
