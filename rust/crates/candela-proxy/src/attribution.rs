//! Multi-dimensional cost attribution (Tenant ID, Job ID) extraction.
//!
//! This module implements the logic for extracting attribution metadata from
//! incoming HTTP requests using W3C Baggage and custom headers.
//!
//! Ported from: `cmd/candela-local/span_capture.go`

use axum::http::HeaderMap;
use std::collections::HashMap;

/// Attribution metadata extracted from a request.
#[derive(Debug, Default, Clone)]
pub struct Attribution {
    pub tenant_id: Option<String>,
    pub job_id: Option<String>,
}

impl Attribution {
    /// Extract attribution from request headers.
    /// Checks explicit headers first, then falls back to W3C Baggage.
    pub fn from_headers(headers: &HeaderMap) -> Self {
        let mut attr = Self::default();

        // 1. Explicit headers (highest priority)
        if let Some(tenant_id) = headers.get("X-Candela-Tenant-Id")
            && let Ok(s) = tenant_id.to_str()
        {
            attr.tenant_id = Some(s.to_string());
        }
        if let Some(job_id) = headers.get("X-Candela-Job-Id")
            && let Ok(s) = job_id.to_str()
        {
            attr.job_id = Some(s.to_string());
        }

        // 2. W3C Baggage fallback
        if (attr.tenant_id.is_none() || attr.job_id.is_none())
            && let Some(baggage) = headers.get("Baggage")
            && let Ok(s) = baggage.to_str()
        {
            let parsed = parse_baggage(s);
            if attr.tenant_id.is_none() {
                attr.tenant_id = parsed.get("candela.tenant_id").cloned();
            }
            if attr.job_id.is_none() {
                attr.job_id = parsed.get("candela.job_id").cloned();
            }
        }

        attr
    }
}

/// Simple W3C Baggage parser.
/// Format: key1=val1,key2=val2;prop1=v1
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
            map.insert(k.trim().to_string(), v.trim().to_string());
        }
    }
    map
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::http::HeaderValue;

    #[test]
    fn test_extract_from_headers() {
        let mut headers = HeaderMap::new();
        headers.insert("X-Candela-Tenant-Id", HeaderValue::from_static("acme"));
        headers.insert("X-Candela-Job-Id", HeaderValue::from_static("job-123"));

        let attr = Attribution::from_headers(&headers);
        assert_eq!(attr.tenant_id, Some("acme".to_string()));
        assert_eq!(attr.job_id, Some("job-123".to_string()));
    }

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

    #[test]
    fn test_explicit_headers_override_baggage() {
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
        assert_eq!(attr.tenant_id, Some("header-tenant".to_string()));
        assert_eq!(attr.job_id, Some("job-789".to_string()));
    }
}
