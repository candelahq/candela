//! Candela Enrichment SDK for Rust.
//!
//! Zero-dependency middleware for propagating tenant and job metadata
//! to Candela AI observability proxies via W3C Baggage headers.
//!
//! # Usage with reqwest
//!
//! ```rust,no_run
//! use candela_sdk::CandelaSession;
//!
//! let session = CandelaSession::builder()
//!     .tenant_id("acme-corp")
//!     .job_id("training-v3")
//!     .build()
//!     .unwrap();
//!
//! let client = reqwest::Client::new();
//! let mut req = client
//!     .post("http://localhost:8080/v1/chat/completions")
//!     .build()
//!     .unwrap();
//! session.inject_headers(req.headers_mut());
//! ```

use std::collections::HashMap;
use std::fmt;

/// Allowed characters for tenant and job IDs: alphanumeric, hyphens, dots, underscores.
fn is_valid_id(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 128
        && s.chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.' || c == '_')
}

/// Error returned when an invalid ID is provided.
#[derive(Debug, Clone)]
pub struct InvalidIdError {
    name: String,
    value: String,
}

impl fmt::Display for InvalidIdError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "candela: invalid {} {:?} — must be 1-128 chars of [a-zA-Z0-9._-]",
            self.name, self.value
        )
    }
}

impl std::error::Error for InvalidIdError {}

/// Validate a tenant or job ID.
pub fn validate_id(value: &str, name: &str) -> Result<(), InvalidIdError> {
    if is_valid_id(value) {
        Ok(())
    } else {
        Err(InvalidIdError {
            name: name.to_string(),
            value: value.to_string(),
        })
    }
}

/// Builder for [`CandelaSession`].
#[derive(Default)]
pub struct SessionBuilder {
    tenant_id: Option<String>,
    job_id: Option<String>,
}

impl SessionBuilder {
    /// Set the tenant ID.
    pub fn tenant_id(mut self, id: &str) -> Self {
        self.tenant_id = Some(id.to_string());
        self
    }

    /// Set the job/experiment ID.
    pub fn job_id(mut self, id: &str) -> Self {
        self.job_id = Some(id.to_string());
        self
    }

    /// Build the session, validating all IDs.
    pub fn build(self) -> Result<CandelaSession, InvalidIdError> {
        if let Some(ref id) = self.tenant_id {
            validate_id(id, "tenant_id")?;
        }
        if let Some(ref id) = self.job_id {
            validate_id(id, "job_id")?;
        }
        Ok(CandelaSession {
            tenant_id: self.tenant_id,
            job_id: self.job_id,
        })
    }
}

/// Reusable session that generates enrichment headers for all requests.
#[derive(Debug, Clone)]
pub struct CandelaSession {
    tenant_id: Option<String>,
    job_id: Option<String>,
}

impl CandelaSession {
    /// Create a new builder.
    pub fn builder() -> SessionBuilder {
        SessionBuilder::default()
    }

    /// Return enrichment headers as a `HashMap`.
    pub fn headers(&self) -> HashMap<String, String> {
        let mut h = HashMap::new();
        let mut parts = Vec::new();

        if let Some(ref id) = self.tenant_id {
            parts.push(format!("candela.tenant_id={}", id));
            h.insert("X-Candela-Tenant-Id".to_string(), id.clone());
        }

        if let Some(ref id) = self.job_id {
            parts.push(format!("candela.job_id={}", id));
            h.insert("X-Candela-Job-Id".to_string(), id.clone());
        }

        if !parts.is_empty() {
            h.insert("Baggage".to_string(), parts.join(","));
        }

        h
    }

    /// Inject enrichment headers into an `http::HeaderMap`.
    ///
    /// Uses `append` for the Baggage header to preserve existing entries
    /// from other instrumentation. For use with reqwest, hyper, etc.
    #[cfg(feature = "http")]
    pub fn inject_headers(&self, headers: &mut http::HeaderMap) {
        for (k, v) in self.headers() {
            if let (Ok(name), Ok(val)) = (
                http::header::HeaderName::from_bytes(k.as_bytes()),
                http::header::HeaderValue::from_str(&v),
            ) {
                if name.as_str().eq_ignore_ascii_case("baggage") {
                    headers.append(name, val);
                } else {
                    headers.insert(name, val);
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn valid_ids() {
        assert!(is_valid_id("acme-corp"));
        assert!(is_valid_id("run_42"));
        assert!(is_valid_id("a.b.c"));
        assert!(is_valid_id(&"A".repeat(128)));
    }

    #[test]
    fn invalid_ids() {
        assert!(!is_valid_id(""));
        assert!(!is_valid_id("has spaces"));
        assert!(!is_valid_id(&"A".repeat(129)));
        assert!(!is_valid_id("bad!char"));
    }

    #[test]
    fn session_headers_both_ids() {
        let s = CandelaSession::builder()
            .tenant_id("acme")
            .job_id("run-1")
            .build()
            .unwrap();

        let h = s.headers();
        assert_eq!(h.get("X-Candela-Tenant-Id").unwrap(), "acme");
        assert_eq!(h.get("X-Candela-Job-Id").unwrap(), "run-1");

        let baggage = h.get("Baggage").unwrap();
        assert!(baggage.contains("candela.tenant_id=acme"));
        assert!(baggage.contains("candela.job_id=run-1"));
    }

    #[test]
    fn session_headers_tenant_only() {
        let s = CandelaSession::builder()
            .tenant_id("t1")
            .build()
            .unwrap();

        let h = s.headers();
        assert_eq!(h.get("X-Candela-Tenant-Id").unwrap(), "t1");
        assert!(!h.contains_key("X-Candela-Job-Id"));
    }

    #[test]
    fn session_empty() {
        let s = CandelaSession::builder().build().unwrap();
        let h = s.headers();
        assert!(!h.contains_key("Baggage"));
    }

    #[test]
    fn builder_rejects_invalid() {
        let result = CandelaSession::builder()
            .tenant_id("bad spaces!")
            .build();
        assert!(result.is_err());
    }

    #[test]
    fn headers_are_fresh() {
        let s = CandelaSession::builder()
            .tenant_id("t1")
            .build()
            .unwrap();

        let h1 = s.headers();
        let h2 = s.headers();
        assert_eq!(h1, h2);
    }
}
