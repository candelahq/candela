//! Multi-dimensional cost attribution (Tenant ID, Job ID) extraction.
//!
//! This module implements the logic for extracting attribution metadata from
//! incoming HTTP requests using W3C Baggage and custom headers.
//!
//! Ported from: `cmd/candela-local/span_capture.go`
//!
//! ## Precedence
//!
//! **Baggage wins over explicit headers** — Baggage is set at the application
//! boundary and propagated automatically by ADK/OTel through every LLM call
//! in the agent trace tree, correctly representing the tenant for the ENTIRE
//! trace — not just one hop. The explicit header is the fallback for non-OTel
//! callers (scripts, direct API use).

use axum::http::HeaderMap;
use regex::Regex;
use std::collections::HashMap;
use std::sync::LazyLock;

/// Validates tenant/job IDs: alphanumeric, hyphens, dots, underscores, 1-128 chars.
/// Prevents log injection and trace poisoning via crafted header values.
/// Must match Go's `tenantIDPattern` in proxy.go.
static ATTRIBUTION_ID_PATTERN: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^[a-zA-Z0-9\-._]{1,128}$").unwrap());

/// Attribution metadata extracted from a request.
#[derive(Debug, Default, Clone)]
pub struct Attribution {
    pub tenant_id: Option<String>,
    pub job_id: Option<String>,
}

impl Attribution {
    /// Extract attribution from request headers.
    ///
    /// Precedence: W3C Baggage > explicit headers (matches Go proxy).
    /// All values are validated against `ATTRIBUTION_ID_PATTERN`.
    pub fn from_headers(headers: &HeaderMap) -> Self {
        let mut attr = Self::default();

        // 1. W3C Baggage (highest priority — matches Go proxy behavior).
        //    Join ALL Baggage header instances (W3C allows multiple).
        let baggage_values: Vec<&str> = headers
            .get_all("Baggage")
            .iter()
            .filter_map(|v| v.to_str().ok())
            .collect();

        if !baggage_values.is_empty() {
            let joined = baggage_values.join(",");
            let parsed = parse_baggage(&joined);
            if let Some(tid) = parsed.get("candela.tenant_id") {
                attr.tenant_id = validate_id(tid);
            }
            if let Some(jid) = parsed.get("candela.job_id") {
                attr.job_id = validate_id(jid);
            }
        }

        // 2. Explicit headers (fallback for non-OTel callers).
        if attr.tenant_id.is_none()
            && let Some(tenant_id) = headers.get("X-Candela-Tenant-Id")
            && let Ok(s) = tenant_id.to_str()
        {
            attr.tenant_id = validate_id(s);
        }
        if attr.job_id.is_none()
            && let Some(job_id) = headers.get("X-Candela-Job-Id")
            && let Ok(s) = job_id.to_str()
        {
            attr.job_id = validate_id(s);
        }

        attr
    }
}

/// Validate an attribution ID against the pattern.
/// Returns None for invalid values (log injection, path traversal, etc.).
fn validate_id(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if ATTRIBUTION_ID_PATTERN.is_match(trimmed) {
        Some(trimmed.to_string())
    } else {
        if !trimmed.is_empty() {
            tracing::warn!(value = trimmed, "discarding invalid attribution ID");
        }
        None
    }
}

/// Simple W3C Baggage parser.
/// Format: key1=val1,key2=val2;prop1=v1
///
/// Keys are matched case-insensitively per RFC 8941.
fn parse_baggage(baggage: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for entry in baggage.split(',') {
        let entry = entry.trim();
        if entry.is_empty() {
            continue;
        }

        // Split by semicolon to remove optional properties
        let part = entry.split(';').next().unwrap_or(entry);
        let mut kv = part.splitn(2, '=');
        if let (Some(k), Some(v)) = (kv.next(), kv.next()) {
            // RFC 8941: keys are case-insensitive — normalize to lowercase.
            map.insert(k.trim().to_lowercase(), v.trim().to_string());
        }
    }
    map
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::http::HeaderValue;

    // ── CRIT-1: Baggage takes precedence over explicit headers ──

    #[test]
    fn baggage_wins_over_explicit_header() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "X-Candela-Tenant-Id",
            HeaderValue::from_static("header-tenant"),
        );
        headers.insert(
            "Baggage",
            HeaderValue::from_static("candela.tenant_id=baggage-tenant, candela.job_id=job-789"),
        );

        let attr = Attribution::from_headers(&headers);
        assert_eq!(
            attr.tenant_id,
            Some("baggage-tenant".to_string()),
            "Baggage must win over explicit header"
        );
        assert_eq!(attr.job_id, Some("job-789".to_string()));
    }

    #[test]
    fn explicit_header_used_when_no_baggage() {
        let mut headers = HeaderMap::new();
        headers.insert("X-Candela-Tenant-Id", HeaderValue::from_static("acme"));
        headers.insert("X-Candela-Job-Id", HeaderValue::from_static("job-123"));

        let attr = Attribution::from_headers(&headers);
        assert_eq!(attr.tenant_id, Some("acme".to_string()));
        assert_eq!(attr.job_id, Some("job-123".to_string()));
    }

    // ── CRIT-2: Tenant ID validation ──

    #[test]
    fn rejects_path_traversal_tenant_id() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "X-Candela-Tenant-Id",
            HeaderValue::from_static("../../etc/passwd"),
        );
        let attr = Attribution::from_headers(&headers);
        assert_eq!(attr.tenant_id, None, "path traversal must be rejected");
    }

    #[test]
    fn rejects_space_in_tenant_id() {
        let mut headers = HeaderMap::new();
        headers.insert("X-Candela-Tenant-Id", HeaderValue::from_static("acme corp"));
        let attr = Attribution::from_headers(&headers);
        assert_eq!(attr.tenant_id, None, "spaces must be rejected");
    }

    #[test]
    fn accepts_valid_tenant_patterns() {
        for id in &["acme-corp", "tenant_42", "dot.tenant", "A1b2C3"] {
            let mut headers = HeaderMap::new();
            headers.insert("X-Candela-Tenant-Id", HeaderValue::from_str(id).unwrap());
            let attr = Attribution::from_headers(&headers);
            assert_eq!(attr.tenant_id, Some(id.to_string()), "should accept {id}");
        }
    }

    // ── CRIT-3: Case-insensitive baggage key matching ──

    #[test]
    fn baggage_key_case_insensitive() {
        for header in &[
            "Candela.Tenant_Id=acme-corp",
            "CANDELA.TENANT_ID=acme-corp",
            "candela.TENANT_ID=acme-corp",
        ] {
            let mut headers = HeaderMap::new();
            headers.insert("Baggage", HeaderValue::from_str(header).unwrap());
            let attr = Attribution::from_headers(&headers);
            assert_eq!(
                attr.tenant_id,
                Some("acme-corp".to_string()),
                "key matching must be case-insensitive for: {header}"
            );
        }
    }

    // ── CRIT-5: Multiple Baggage header instances ──

    #[test]
    fn multiple_baggage_headers_joined() {
        let mut headers = HeaderMap::new();
        headers.append("Baggage", HeaderValue::from_static("svc.name=my-service"));
        headers.append(
            "Baggage",
            HeaderValue::from_static("candela.tenant_id=multi-tenant"),
        );

        let attr = Attribution::from_headers(&headers);
        assert_eq!(
            attr.tenant_id,
            Some("multi-tenant".to_string()),
            "must join multiple Baggage header instances"
        );
    }

    // ── Existing tests preserved ──

    #[test]
    fn test_extract_from_baggage() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "Baggage",
            HeaderValue::from_static("candela.tenant_id=acme, candela.job_id=job-456;prop=1"),
        );

        let attr = Attribution::from_headers(&headers);
        assert_eq!(attr.tenant_id, Some("acme".to_string()));
        assert_eq!(attr.job_id, Some("job-456".to_string()));
    }
}
