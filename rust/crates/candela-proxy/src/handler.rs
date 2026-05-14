//! Axum HTTP handler for the LLM reverse proxy.
//!
//! Implements the core request lifecycle:
//!   1. Extract provider name from URL path (`/proxy/{provider}/...`)
//!   2. Buffer + optionally translate the request body
//!   3. Forward to upstream with circuit-breaker gating
//!   4. Capture response (streaming or non-streaming)
//!   5. Build an observability [`Span`] and submit to the pipeline
//!
//! Ported from: `pkg/proxy/proxy.go` — `ServeHTTP` / `handleRequest`

use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::Instant;

use axum::body::Body;
use axum::extract::{Path, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::{IntoResponse, Response};
use bytes::Bytes;
use chrono::Utc;
use regex::Regex;
use tracing::{error, info, warn};

use candela_core::{GenAIAttributes, Span, SpanKind, SpanStatus};

use crate::ids::{new_span_id, new_trace_id};
use crate::parsers::{self, CacheTokens};
use crate::{Proxy, SpanSubmitter};

/// Maximum request body size (10 MB) — matches Go's MaxBytesReader.
const MAX_REQUEST_BODY: usize = 10 << 20;

/// Validates request/session IDs: alphanumeric + hyphens/dots/underscores, 1-128 chars.
/// Prevents log injection and trace poisoning.
fn validate_request_id(id: &str) -> bool {
    static RE: std::sync::LazyLock<Regex> =
        std::sync::LazyLock::new(|| Regex::new(r"^[a-zA-Z0-9\-._]{1,128}$").unwrap());
    RE.is_match(id)
}

/// Parsed W3C Trace Context from a `Traceparent` header.
struct TraceContext {
    trace_id: String,
    parent_span_id: String,
}

/// Parse a W3C `Traceparent` header: `{version}-{trace-id}-{parent-id}-{flags}`.
fn parse_traceparent(header: &str) -> Option<TraceContext> {
    let parts: Vec<&str> = header.split('-').collect();
    if parts.len() != 4 {
        return None;
    }
    let trace_id = parts[1];
    let parent_id = parts[2];
    if trace_id.len() != 32 || parent_id.len() != 16 {
        return None;
    }
    if !trace_id.chars().all(|c| c.is_ascii_hexdigit())
        || !parent_id.chars().all(|c| c.is_ascii_hexdigit())
    {
        return None;
    }
    Some(TraceContext {
        trace_id: trace_id.to_string(),
        parent_span_id: parent_id.to_string(),
    })
}

/// Emit cache token counts as span attributes when non-zero.
fn add_cache_attrs(attrs: &mut BTreeMap<String, String>, cache: &CacheTokens) {
    if cache.cache_read_tokens > 0 {
        attrs.insert(
            "gen_ai.usage.cache_read_tokens".into(),
            cache.cache_read_tokens.to_string(),
        );
    }
    if cache.cache_creation_tokens > 0 {
        attrs.insert(
            "gen_ai.usage.cache_creation_tokens".into(),
            cache.cache_creation_tokens.to_string(),
        );
    }
}

/// Shared application state passed to axum handlers via `State`.
pub struct AppState {
    pub proxy: Arc<Proxy>,
    pub submitter: Arc<dyn SpanSubmitter>,
    /// Shared HTTP client — reused across requests for connection pooling.
    pub http_client: reqwest::Client,
}

/// Primary proxy handler — mounted at `/proxy/{provider}/*rest`.
///
/// Mirrors the Go `Proxy.ServeHTTP` method:
///   - Routes by provider name
///   - Optionally translates request/response formats
///   - Captures token usage and generates an observability span
pub async fn proxy_handler(
    State(state): State<Arc<AppState>>,
    Path((provider_name, rest)): Path<(String, String)>,
    headers: HeaderMap,
    body: Body,
) -> Response {
    let start = Instant::now();
    let start_time = Utc::now();

    // ── 1. Resolve provider ──
    let provider = match state.proxy.get_provider(&provider_name) {
        Some(p) => p,
        None => {
            warn!(provider = %provider_name, "unknown provider");
            return (
                StatusCode::BAD_GATEWAY,
                format!("unknown provider: {provider_name}"),
            )
                .into_response();
        }
    };

    // ── 2. Circuit breaker check ──
    if !state.proxy.check_circuit(&provider_name).await {
        warn!(provider = %provider_name, "circuit breaker open");
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            "provider circuit breaker is open — try again later",
        )
            .into_response();
    }

    // ── 3. Buffer request body (with size limit) ──
    let request_bytes = match axum::body::to_bytes(body, MAX_REQUEST_BODY).await {
        Ok(bytes) => bytes,
        Err(e) => {
            error!(error = %e, "failed to read request body");
            return (StatusCode::BAD_REQUEST, "failed to read request body").into_response();
        }
    };

    // ── 4. Parse model from request + optionally translate ──
    let (upstream_body, model) = if let Some(ref translator) = provider.format_translator {
        match translator.translate_request(&request_bytes) {
            Ok((translated, model)) => (Bytes::from(translated), model),
            Err(e) => {
                error!(error = %e, provider = %provider_name, "request translation failed");
                return (StatusCode::BAD_REQUEST, "request translation failed").into_response();
            }
        }
    } else {
        // Passthrough — extract model from JSON body.
        let model = extract_model(&request_bytes).unwrap_or_default();
        (request_bytes.clone(), model)
    };

    // ── 5. Detect streaming ──
    let is_streaming = detect_streaming(&request_bytes);

    // ── 6. Build upstream URL ──
    // SECURITY: Sanitize the rest path to prevent directory traversal.
    let sanitized_rest = sanitize_path(&rest);
    let upstream_path = if let Some(ref rewriter) = provider.path_rewriter {
        rewriter.rewrite_path(&model, is_streaming)
    } else {
        format!("/{sanitized_rest}")
    };
    let upstream_url = format!(
        "{}{upstream_path}",
        provider.upstream_url.trim_end_matches('/')
    );

    // ── 7. Forward request to upstream ──
    // Reuse the shared HTTP client for connection pooling.
    let mut upstream_req = state.http_client.post(&upstream_url).body(upstream_body);

    // Forward relevant headers (auth, content-type, accept).
    for key in [
        "authorization",
        "content-type",
        "accept",
        "anthropic-version",
        "x-api-key",
    ] {
        if let Some(v) = headers.get(key).and_then(|val| val.to_str().ok()) {
            upstream_req = upstream_req.header(key, v);
        }
    }

    let upstream_result = upstream_req.send().await;

    let (response_status, response_bytes, upstream_content_type) = match upstream_result {
        Ok(resp) => {
            let status = resp.status();
            // Capture upstream Content-Type before consuming the body.
            let content_type = resp
                .headers()
                .get("content-type")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("application/json")
                .to_string();
            let body_bytes = match resp.bytes().await {
                Ok(b) => b,
                Err(e) => {
                    error!(error = %e, "failed to read upstream response body");
                    state.proxy.record_failure(&provider_name).await;
                    return (
                        StatusCode::BAD_GATEWAY,
                        "failed to read upstream response body",
                    )
                        .into_response();
                }
            };

            if status.is_success() {
                state.proxy.record_success(&provider_name).await;
            } else {
                state.proxy.record_failure(&provider_name).await;
            }

            (status, body_bytes, content_type)
        }
        Err(e) => {
            error!(error = %e, provider = %provider_name, upstream = %upstream_url, "upstream request failed");
            state.proxy.record_failure(&provider_name).await;
            return (StatusCode::BAD_GATEWAY, format!("upstream error: {e}")).into_response();
        }
    };

    // ── 8. Optionally translate response ──
    let client_body = if let Some(ref translator) = provider.format_translator {
        match translator.translate_response(&response_bytes, &model) {
            Ok(translated) => Bytes::from(translated),
            Err(e) => {
                warn!(error = %e, "response translation failed, returning raw");
                response_bytes.clone()
            }
        }
    } else {
        response_bytes.clone()
    };

    // ── 9. Extract token usage (provider-aware) ──
    let (output_content_parsed, usage) =
        parsers::extract_response_usage(&provider_name, &response_bytes);
    let cache_tokens = parsers::extract_cache_tokens(&provider_name, &response_bytes);

    // ── 10. Build span ──
    let elapsed = start.elapsed();
    let end_time = Utc::now();

    // Truncate at byte boundary first to avoid decoding multi-MB bodies.
    let input_content = String::from_utf8_lossy(&request_bytes[..request_bytes.len().min(16384)])
        .chars()
        .take(4096)
        .collect::<String>();
    let output_content = if output_content_parsed.is_empty() {
        String::from_utf8_lossy(&response_bytes[..response_bytes.len().min(16384)])
            .chars()
            .take(4096)
            .collect::<String>()
    } else {
        output_content_parsed.chars().take(4096).collect::<String>()
    };

    let span_status = if response_status.is_success() {
        SpanStatus::Ok
    } else {
        SpanStatus::Error
    };

    let status_message = if !response_status.is_success() {
        Some(format!("HTTP {}", response_status.as_u16()))
    } else {
        None
    };

    // ── W3C Trace Context ──
    let trace_ctx =
        extract_header_str(&headers, "traceparent").and_then(|tp| parse_traceparent(&tp));

    // Generate or validate request ID.
    let request_id = extract_header_str(&headers, "x-request-id")
        .filter(|id| validate_request_id(id))
        .unwrap_or_else(new_trace_id);

    // Extract user/tenant/session from headers or baggage.
    let user_id = extract_header_str(&headers, "x-user-id");
    let session_id =
        extract_header_str(&headers, "x-session-id").filter(|id| validate_request_id(id));
    let tenant_id = extract_baggage_value(&headers, "candela.tenant_id");
    let job_id = extract_baggage_value(&headers, "candela.job_id");

    let mut attributes = BTreeMap::new();
    attributes.insert("http.method".into(), "POST".into());
    attributes.insert("http.url".into(), upstream_url.clone());
    attributes.insert(
        "http.status_code".into(),
        response_status.as_u16().to_string(),
    );
    attributes.insert("http.ttfb_ms".into(), elapsed.as_millis().to_string());
    attributes.insert("http.request_id".into(), request_id.clone());
    attributes.insert("llm.provider".into(), provider_name.clone());
    if is_streaming {
        attributes.insert("llm.streaming".into(), "true".into());
    }
    if let Some(ref tid) = tenant_id {
        attributes.insert("candela.tenant_id".into(), tid.clone());
    }
    add_cache_attrs(&mut attributes, &cache_tokens);

    // Determine trace/span IDs from W3C context or generate new ones.
    let (trace_id, parent_span_id) = if let Some(ref ctx) = trace_ctx {
        (ctx.trace_id.clone(), Some(ctx.parent_span_id.clone()))
    } else {
        (
            extract_header_str(&headers, "x-trace-id").unwrap_or_else(new_trace_id),
            extract_header_str(&headers, "x-parent-span-id"),
        )
    };

    let span = Span {
        span_id: new_span_id(),
        trace_id,
        parent_span_id,
        name: format!("llm.{provider_name}.chat"),
        kind: SpanKind::Llm,
        status: span_status,
        status_message,
        start_time,
        end_time,
        duration: elapsed,
        gen_ai: if model.is_empty() {
            None
        } else {
            Some(GenAIAttributes {
                model: model.clone(),
                provider: provider_name.clone(),
                input_tokens: usage.input_tokens,
                output_tokens: usage.output_tokens,
                total_tokens: usage.total_tokens,
                input_content,
                output_content,
                cache_read_tokens: cache_tokens.cache_read_tokens,
                cache_creation_tokens: cache_tokens.cache_creation_tokens,
                ..Default::default()
            })
        },
        attributes,
        project_id: state.proxy.project_id().to_string(),
        environment: extract_header_str(&headers, "x-environment"),
        service_name: extract_header_str(&headers, "x-service-name"),
        user_id,
        session_id,
        tenant_id,
        job_id,
    };

    // ── 11. Submit span asynchronously ──
    let submitter = Arc::clone(&state.submitter);
    tokio::spawn(async move {
        submitter.submit_batch(vec![span]);
    });

    info!(
        provider = %provider_name,
        model = %model,
        status = %response_status.as_u16(),
        duration_ms = %elapsed.as_millis(),
        input_tokens = usage.input_tokens,
        output_tokens = usage.output_tokens,
        "proxied request"
    );

    // ── 12. Return response to client ──
    // Forward the upstream Content-Type header instead of hardcoding.
    let mut response = Response::builder()
        .status(StatusCode::from_u16(response_status.as_u16()).unwrap_or(StatusCode::OK));

    response = response.header("content-type", upstream_content_type);
    response = response.header("x-request-id", &request_id);

    response
        .body(Body::from(client_body))
        .unwrap_or_else(|_| StatusCode::INTERNAL_SERVER_ERROR.into_response())
}

// ── Helper Functions ──

/// Extract the `model` field from a JSON request body.
fn extract_model(body: &[u8]) -> Option<String> {
    serde_json::from_slice::<serde_json::Value>(body)
        .ok()?
        .get("model")?
        .as_str()
        .map(String::from)
}

/// Detect if the request has `"stream": true`.
fn detect_streaming(body: &[u8]) -> bool {
    serde_json::from_slice::<serde_json::Value>(body)
        .ok()
        .and_then(|v| v.get("stream")?.as_bool())
        .unwrap_or(false)
}

/// Extract a string header value.
fn extract_header_str(headers: &HeaderMap, key: &str) -> Option<String> {
    headers
        .get(key)
        .and_then(|v| v.to_str().ok())
        .map(String::from)
}

/// Extract a value from W3C Baggage header(s).
///
/// Handles multiple Baggage headers (per W3C spec) and case-insensitive
/// key matching for `candela.tenant_id`.
#[allow(clippy::collapsible_if)]
fn extract_baggage_value(headers: &HeaderMap, key: &str) -> Option<String> {
    let key_lower = key.to_lowercase();
    for val in headers.get_all("baggage") {
        if let Ok(s) = val.to_str() {
            for member in s.split(',') {
                let member = member.trim();
                if let Some((k, v)) = member.split_once('=') {
                    if k.trim().to_lowercase() == key_lower {
                        return Some(v.trim().to_string());
                    }
                }
            }
        }
    }
    None
}

/// Sanitize a URL path segment to prevent directory traversal attacks.
/// URL-decodes first to catch %2e%2e encoded traversal.
fn sanitize_path(path: &str) -> String {
    let decoded = urldecode(path);
    decoded
        .split('/')
        .filter(|seg| !seg.is_empty() && *seg != ".." && *seg != ".")
        .collect::<Vec<_>>()
        .join("/")
}

/// Minimal URL percent-decoding for path sanitization.
fn urldecode(s: &str) -> String {
    let mut result = Vec::with_capacity(s.len());
    let mut bytes = s.as_bytes().iter();
    while let Some(&b) = bytes.next() {
        if b == b'%' {
            let hi = bytes.next().copied().unwrap_or(b'0');
            let lo = bytes.next().copied().unwrap_or(b'0');
            let decoded = (hex_val(hi) << 4) | hex_val(lo);
            result.push(decoded);
        } else {
            result.push(b);
        }
    }
    String::from_utf8_lossy(&result).into_owned()
}

fn hex_val(b: u8) -> u8 {
    match b {
        b'0'..=b'9' => b - b'0',
        b'a'..=b'f' => b - b'a' + 10,
        b'A'..=b'F' => b - b'A' + 10,
        _ => 0,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extract_model_from_openai_body() {
        let body = br#"{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}"#;
        assert_eq!(extract_model(body), Some("gpt-4o".into()));
    }

    #[test]
    fn extract_model_missing() {
        let body = br#"{"messages": []}"#;
        assert_eq!(extract_model(body), None);
    }

    #[test]
    fn extract_model_invalid_json() {
        let body = b"not json";
        assert_eq!(extract_model(body), None);
    }

    #[test]
    fn detect_streaming_true() {
        let body = br#"{"model": "gpt-4", "stream": true}"#;
        assert!(detect_streaming(body));
    }

    #[test]
    fn detect_streaming_false() {
        let body = br#"{"model": "gpt-4", "stream": false}"#;
        assert!(!detect_streaming(body));
    }

    #[test]
    fn detect_streaming_absent() {
        let body = br#"{"model": "gpt-4"}"#;
        assert!(!detect_streaming(body));
    }

    #[test]
    fn extract_openai_token_usage() {
        let body =
            br#"{"usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150}}"#;
        let (_, usage) = parsers::extract_response_usage("openai", body);
        assert_eq!(usage.input_tokens, 100);
        assert_eq!(usage.output_tokens, 50);
        assert_eq!(usage.total_tokens, 150);
    }

    #[test]
    fn extract_anthropic_token_usage() {
        let body = br#"{"usage": {"input_tokens": 200, "output_tokens": 80}}"#;
        let (_, usage) = parsers::extract_response_usage("anthropic", body);
        assert_eq!(usage.input_tokens, 200);
        assert_eq!(usage.output_tokens, 80);
    }

    #[test]
    fn extract_token_usage_missing() {
        let body = br#"{"choices": []}"#;
        let (_, usage) = parsers::extract_response_usage("openai", body);
        assert_eq!(usage.input_tokens, 0);
    }

    #[test]
    fn extract_baggage_single_header() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "baggage",
            "candela.tenant_id=acme-corp,other=val".parse().unwrap(),
        );
        assert_eq!(
            extract_baggage_value(&headers, "candela.tenant_id"),
            Some("acme-corp".into())
        );
    }

    #[test]
    fn extract_baggage_case_insensitive() {
        let mut headers = HeaderMap::new();
        headers.insert("baggage", "Candela.Tenant_ID=ACME".parse().unwrap());
        assert_eq!(
            extract_baggage_value(&headers, "candela.tenant_id"),
            Some("ACME".into())
        );
    }

    #[test]
    fn extract_baggage_multiple_headers() {
        let mut headers = HeaderMap::new();
        headers.append("baggage", "foo=bar".parse().unwrap());
        headers.append("baggage", "candela.tenant_id=multi-corp".parse().unwrap());
        assert_eq!(
            extract_baggage_value(&headers, "candela.tenant_id"),
            Some("multi-corp".into())
        );
    }

    #[test]
    fn extract_baggage_missing() {
        let headers = HeaderMap::new();
        assert_eq!(extract_baggage_value(&headers, "candela.tenant_id"), None);
    }

    #[test]
    fn extract_baggage_job_id() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "baggage",
            "candela.tenant_id=acme,candela.job_id=exp-42"
                .parse()
                .unwrap(),
        );
        assert_eq!(
            extract_baggage_value(&headers, "candela.job_id"),
            Some("exp-42".into())
        );
    }

    #[test]
    fn extract_header_str_present() {
        let mut headers = HeaderMap::new();
        headers.insert("x-user-id", "alice@example.com".parse().unwrap());
        assert_eq!(
            extract_header_str(&headers, "x-user-id"),
            Some("alice@example.com".into())
        );
    }

    #[test]
    fn extract_header_str_absent() {
        let headers = HeaderMap::new();
        assert_eq!(extract_header_str(&headers, "x-user-id"), None);
    }

    // ── W3C Trace Context tests ──

    #[test]
    fn parse_traceparent_valid() {
        let ctx = parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01");
        assert!(ctx.is_some());
        let ctx = ctx.unwrap();
        assert_eq!(ctx.trace_id, "4bf92f3577b34da6a3ce929d0e0e4736");
        assert_eq!(ctx.parent_span_id, "00f067aa0ba902b7");
    }

    #[test]
    fn parse_traceparent_invalid_format() {
        assert!(parse_traceparent("not-a-traceparent").is_none());
        assert!(parse_traceparent("").is_none());
        assert!(parse_traceparent("00-short-id-01").is_none());
    }

    #[test]
    fn parse_traceparent_non_hex() {
        assert!(
            parse_traceparent("00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-00f067aa0ba902b7-01").is_none()
        );
    }

    #[test]
    fn parse_traceparent_extra_dashes() {
        assert!(parse_traceparent("00-aa-bb-cc-dd").is_none());
    }

    #[test]
    fn parse_traceparent_all_zeros() {
        let tp = "00-00000000000000000000000000000000-0000000000000000-00";
        let ctx = parse_traceparent(tp);
        assert!(ctx.is_some());
        assert_eq!(ctx.unwrap().trace_id, "00000000000000000000000000000000");
    }

    // ── Request ID validation tests ──

    #[test]
    fn validate_request_id_accepts_valid() {
        assert!(validate_request_id("abc-123"));
        assert!(validate_request_id("a1b2c3d4e5f6"));
    }

    #[test]
    fn validate_request_id_with_dots_and_underscores() {
        assert!(validate_request_id("req.id_123"));
        assert!(validate_request_id("opencode_trace.v2"));
    }

    #[test]
    fn validate_request_id_at_boundary() {
        let max = "a".repeat(128);
        assert!(validate_request_id(&max));
        let over = "a".repeat(129);
        assert!(!validate_request_id(&over));
    }

    #[test]
    fn validate_request_id_rejects_injection() {
        assert!(!validate_request_id("../../etc/passwd"));
        assert!(!validate_request_id("id with spaces"));
        assert!(!validate_request_id(""));
        assert!(!validate_request_id("id;injection"));
        assert!(!validate_request_id("id=value"));
    }

    // ── Cache attrs tests ──

    #[test]
    fn add_cache_attrs_adds_when_nonzero() {
        let mut attrs = BTreeMap::new();
        let cache = CacheTokens {
            cache_read_tokens: 100,
            cache_creation_tokens: 20,
        };
        add_cache_attrs(&mut attrs, &cache);
        assert_eq!(attrs.get("gen_ai.usage.cache_read_tokens").unwrap(), "100");
        assert_eq!(
            attrs.get("gen_ai.usage.cache_creation_tokens").unwrap(),
            "20"
        );
    }

    #[test]
    fn add_cache_attrs_skips_zero() {
        let mut attrs = BTreeMap::new();
        let cache = CacheTokens::default();
        add_cache_attrs(&mut attrs, &cache);
        assert!(!attrs.contains_key("gen_ai.usage.cache_read_tokens"));
    }

    // ── Path sanitization + URL-encoded traversal ──

    #[test]
    fn sanitize_path_removes_traversal() {
        assert_eq!(sanitize_path("../../admin/secret"), "admin/secret");
        assert_eq!(sanitize_path("v1/../../../etc/passwd"), "v1/etc/passwd");
        assert_eq!(
            sanitize_path("./v1/./chat/completions"),
            "v1/chat/completions"
        );
    }

    #[test]
    fn sanitize_path_preserves_normal() {
        assert_eq!(sanitize_path("v1/chat/completions"), "v1/chat/completions");
        assert_eq!(sanitize_path("v1/models"), "v1/models");
    }

    #[test]
    fn sanitize_path_url_encoded_traversal() {
        assert_eq!(sanitize_path("%2e%2e/admin/secret"), "admin/secret");
        assert_eq!(sanitize_path("v1/%2E%2E/%2E%2E/etc"), "v1/etc");
    }

    #[test]
    fn sanitize_path_mixed_encoding() {
        assert_eq!(sanitize_path("../%2e%2e/secret"), "secret");
    }

    // ── urldecode tests ──

    #[test]
    fn urldecode_basic() {
        assert_eq!(urldecode("hello%20world"), "hello world");
        assert_eq!(urldecode("%2e%2e"), "..");
    }

    #[test]
    fn urldecode_empty() {
        assert_eq!(urldecode(""), "");
    }

    #[test]
    fn urldecode_no_encoding() {
        assert_eq!(urldecode("v1/chat/completions"), "v1/chat/completions");
    }

    #[test]
    fn urldecode_uppercase_hex() {
        assert_eq!(urldecode("%2E%2E"), "..");
    }

    #[test]
    fn hex_val_digits() {
        assert_eq!(hex_val(b'0'), 0);
        assert_eq!(hex_val(b'9'), 9);
        assert_eq!(hex_val(b'a'), 10);
        assert_eq!(hex_val(b'f'), 15);
        assert_eq!(hex_val(b'A'), 10);
        assert_eq!(hex_val(b'F'), 15);
        assert_eq!(hex_val(b'z'), 0);
    }

    // ── Baggage edge cases ──

    #[test]
    fn extract_baggage_whitespace_handling() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "baggage",
            " candela.tenant_id = spaced-value , other=x "
                .parse()
                .unwrap(),
        );
        assert_eq!(
            extract_baggage_value(&headers, "candela.tenant_id"),
            Some("spaced-value".into())
        );
    }

    #[test]
    fn extract_baggage_empty_value() {
        let mut headers = HeaderMap::new();
        headers.insert("baggage", "candela.tenant_id=".parse().unwrap());
        assert_eq!(
            extract_baggage_value(&headers, "candela.tenant_id"),
            Some("".into())
        );
    }

    // ── Model extraction edge cases ──

    #[test]
    fn extract_model_nested_body() {
        let body = br#"{"model":"claude-sonnet-4-20250514","max_tokens":1024}"#;
        assert_eq!(extract_model(body), Some("claude-sonnet-4-20250514".into()));
    }

    #[test]
    fn extract_model_numeric_model() {
        let body = br#"{"model": 42}"#;
        assert_eq!(extract_model(body), None);
    }

    // ── Streaming detection edge cases ──

    #[test]
    fn detect_streaming_with_string_true() {
        let body = br#"{"model": "gpt-4", "stream": "true"}"#;
        assert!(!detect_streaming(body));
    }

    #[test]
    fn detect_streaming_null() {
        let body = br#"{"model": "gpt-4", "stream": null}"#;
        assert!(!detect_streaming(body));
    }
}
