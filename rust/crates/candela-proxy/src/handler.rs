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
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::response::{IntoResponse, Response};
use bytes::Bytes;
use chrono::Utc;
use http_body_util::BodyExt;
use tracing::{error, info, warn};

use candela_core::{GenAIAttributes, Span, SpanKind, SpanStatus};

use crate::ids::{new_span_id, new_trace_id};
use crate::parsers::TokenUsage;
use crate::{Proxy, SpanSubmitter};

/// Shared application state passed to axum handlers via `State`.
pub struct AppState {
    pub proxy: Arc<Proxy>,
    pub submitter: Arc<dyn SpanSubmitter>,
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

    // ── 3. Buffer request body ──
    let request_bytes = match body.collect().await {
        Ok(collected) => collected.to_bytes(),
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
    let upstream_path = if let Some(ref rewriter) = provider.path_rewriter {
        rewriter.rewrite_path(&model, is_streaming)
    } else {
        format!("/{rest}")
    };
    let upstream_url = format!(
        "{}{upstream_path}",
        provider.upstream_url.trim_end_matches('/')
    );

    // ── 7. Forward request to upstream ──
    let client = reqwest::Client::new();
    let mut upstream_req = client.post(&upstream_url).body(upstream_body.to_vec());

    // Forward relevant headers (auth, content-type, accept).
    for key in [
        "authorization",
        "content-type",
        "accept",
        "anthropic-version",
        "x-api-key",
    ] {
        if let Some(val) = headers.get(key)
            && let Ok(v) = val.to_str()
        {
            upstream_req = upstream_req.header(key, v);
        }
    }

    let upstream_result = upstream_req.send().await;

    let (response_status, response_bytes) = match upstream_result {
        Ok(resp) => {
            let status = resp.status();
            let body_bytes = resp.bytes().await.unwrap_or_default();

            if status.is_success() {
                state.proxy.record_success(&provider_name).await;
            } else {
                state.proxy.record_failure(&provider_name).await;
            }

            (status, body_bytes)
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

    // ── 9. Extract token usage ──
    let usage = extract_token_usage(&response_bytes);

    // ── 10. Build span ──
    let elapsed = start.elapsed();
    let end_time = Utc::now();

    let input_content = String::from_utf8_lossy(&request_bytes)
        .chars()
        .take(4096)
        .collect::<String>();
    let output_content = String::from_utf8_lossy(&response_bytes)
        .chars()
        .take(4096)
        .collect::<String>();

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

    // Extract user/tenant/session from headers or baggage.
    let user_id = extract_header_str(&headers, "x-user-id");
    let session_id = extract_header_str(&headers, "x-session-id");
    let tenant_id = extract_baggage_value(&headers, "candela.tenant_id");

    let mut attributes = BTreeMap::new();
    attributes.insert("http.method".into(), "POST".into());
    attributes.insert("http.url".into(), upstream_url.clone());
    attributes.insert(
        "http.status_code".into(),
        response_status.as_u16().to_string(),
    );
    attributes.insert("llm.provider".into(), provider_name.clone());
    if is_streaming {
        attributes.insert("llm.streaming".into(), "true".into());
    }
    if let Some(ref tid) = tenant_id {
        attributes.insert("candela.tenant_id".into(), tid.clone());
    }

    let span = Span {
        span_id: new_span_id(),
        trace_id: extract_header_str(&headers, "x-trace-id").unwrap_or_else(new_trace_id),
        parent_span_id: extract_header_str(&headers, "x-parent-span-id"),
        name: format!("llm.{provider_name}.chat"),
        kind: SpanKind::Llm,
        status: span_status,
        status_message,
        start_time,
        end_time,
        duration: elapsed,
        gen_ai: Some(GenAIAttributes {
            model: model.clone(),
            provider: provider_name.clone(),
            input_tokens: usage.input_tokens,
            output_tokens: usage.output_tokens,
            total_tokens: usage.total_tokens,
            input_content,
            output_content,
            ..Default::default()
        }),
        attributes,
        project_id: state.proxy.project_id().to_string(),
        environment: extract_header_str(&headers, "x-environment"),
        service_name: extract_header_str(&headers, "x-service-name"),
        user_id,
        session_id,
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
    let mut response = Response::builder()
        .status(StatusCode::from_u16(response_status.as_u16()).unwrap_or(StatusCode::OK));

    // Preserve content-type from upstream.
    if let Ok(ct) = HeaderValue::from_str("application/json") {
        response = response.header("content-type", ct);
    }

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

/// Extract token usage from upstream JSON response.
fn extract_token_usage(body: &[u8]) -> TokenUsage {
    let val: serde_json::Value = match serde_json::from_slice(body) {
        Ok(v) => v,
        Err(_) => return TokenUsage::default(),
    };

    // OpenAI format: { "usage": { "prompt_tokens": N, "completion_tokens": N, "total_tokens": N } }
    if let Some(usage) = val.get("usage") {
        return TokenUsage {
            input_tokens: usage
                .get("prompt_tokens")
                .or_else(|| usage.get("input_tokens"))
                .and_then(|v| v.as_i64())
                .unwrap_or(0),
            output_tokens: usage
                .get("completion_tokens")
                .or_else(|| usage.get("output_tokens"))
                .and_then(|v| v.as_i64())
                .unwrap_or(0),
            total_tokens: usage
                .get("total_tokens")
                .and_then(|v| v.as_i64())
                .unwrap_or(0),
        };
    }

    TokenUsage::default()
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
fn extract_baggage_value(headers: &HeaderMap, key: &str) -> Option<String> {
    let key_lower = key.to_lowercase();
    for val in headers.get_all("baggage") {
        if let Ok(s) = val.to_str() {
            for member in s.split(',') {
                let member = member.trim();
                if let Some((k, v)) = member.split_once('=')
                    && k.trim().to_lowercase() == key_lower
                {
                    return Some(v.trim().to_string());
                }
            }
        }
    }
    None
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
        let usage = extract_token_usage(body);
        assert_eq!(usage.input_tokens, 100);
        assert_eq!(usage.output_tokens, 50);
        assert_eq!(usage.total_tokens, 150);
    }

    #[test]
    fn extract_anthropic_token_usage() {
        let body = br#"{"usage": {"input_tokens": 200, "output_tokens": 80}}"#;
        let usage = extract_token_usage(body);
        assert_eq!(usage.input_tokens, 200);
        assert_eq!(usage.output_tokens, 80);
    }

    #[test]
    fn extract_token_usage_missing() {
        let body = br#"{"choices": []}"#;
        let usage = extract_token_usage(body);
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
}
