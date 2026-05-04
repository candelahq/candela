//! Span and trace ID generation.
//!
//! Ported from: `pkg/proxy/ids.go`

use uuid::Uuid;

/// Generate a new random trace ID (32-char hex).
pub fn new_trace_id() -> String {
    Uuid::new_v4().simple().to_string()
}

/// Generate a new random span ID (16-char hex).
pub fn new_span_id() -> String {
    let uuid = Uuid::new_v4();
    let bytes = uuid.as_bytes();
    hex::encode(&bytes[..8])
}

// We'll need the `hex` crate — but for now, manual implementation:
mod hex {
    pub fn encode(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn trace_id_is_32_hex_chars() {
        let id = new_trace_id();
        assert_eq!(id.len(), 32);
        assert!(id.chars().all(|c| c.is_ascii_hexdigit()));
    }

    #[test]
    fn span_id_is_16_hex_chars() {
        let id = new_span_id();
        assert_eq!(id.len(), 16);
        assert!(id.chars().all(|c| c.is_ascii_hexdigit()));
    }

    #[test]
    fn ids_are_unique() {
        let a = new_trace_id();
        let b = new_trace_id();
        assert_ne!(a, b);
    }
}
