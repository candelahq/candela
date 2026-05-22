//! Integration test for the Rust proxy handler.
//!
//! Spins up a mock upstream server and the sidecar proxy, then verifies
//! end-to-end request forwarding, span generation, and error handling.

use std::sync::{Arc, Mutex};

use axum::routing::get;
use axum::{Json, Router, routing::post};
use candela_core::Span;
use candela_proxy::handler::{AppState, proxy_handler};
use candela_proxy::{Config, Provider, SpanSubmitter};
use tokio::net::TcpListener;

/// Captures submitted spans for test assertions.
struct TestSubmitter {
    spans: Mutex<Vec<Span>>,
}

impl TestSubmitter {
    fn new() -> Self {
        Self {
            spans: Mutex::new(Vec::new()),
        }
    }

    fn captured_spans(&self) -> Vec<Span> {
        self.spans.lock().unwrap().clone()
    }
}

impl SpanSubmitter for TestSubmitter {
    fn submit_batch(&self, spans: Vec<Span>) {
        self.spans.lock().unwrap().extend(spans);
    }
}

/// Start a mock OpenAI upstream that returns a canned completion response.
async fn start_mock_upstream() -> (String, tokio::task::JoinHandle<()>) {
    let app = Router::new()
        .route(
            "/v1/chat/completions",
            post(|| async {
                Json(serde_json::json!({
                    "id": "chatcmpl-test123",
                    "object": "chat.completion",
                    "model": "gpt-4o",
                    "choices": [{
                        "index": 0,
                        "message": {"role": "assistant", "content": "Hello from mock!"},
                        "finish_reason": "stop"
                    }],
                    "usage": {
                        "prompt_tokens": 10,
                        "completion_tokens": 5,
                        "total_tokens": 15
                    }
                }))
            }),
        )
        .route(
            "/v1/chat/error",
            post(|| async {
                (
                    axum::http::StatusCode::INTERNAL_SERVER_ERROR,
                    Json(serde_json::json!({"error": {"message": "server error"}})),
                )
            }),
        );

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let url = format!("http://127.0.0.1:{}", addr.port());

    let handle = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    (url, handle)
}

/// Start the proxy server pointing at the given upstream URL.
async fn start_proxy(
    upstream_url: &str,
    submitter: Arc<TestSubmitter>,
) -> (String, tokio::task::JoinHandle<()>) {
    let proxy = Arc::new(candela_proxy::Proxy::new(Config {
        providers: vec![Provider {
            name: "openai".into(),
            upstream_url: upstream_url.to_string(),
            host: None,
            host_pattern: None,
            intercept: None,
            mitm: None,
            format_translator: None,
            path_rewriter: None,
        }],
        project_id: "integration-test".into(),
    }));

    let app_state = Arc::new(AppState {
        proxy,
        submitter,
        http_client: reqwest::Client::new(),
    });

    let app = Router::new()
        .route(
            "/proxy/{provider}/{*rest}",
            axum::routing::any(proxy_handler),
        )
        .route("/healthz", get(|| async { "ok" }))
        .with_state(app_state);

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let url = format!("http://127.0.0.1:{}", addr.port());

    let handle = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    // Give the server a moment to start.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;

    (url, handle)
}

/// End-to-end test: OpenAI chat completions round-trip through the proxy.
#[tokio::test]
async fn proxy_openai_round_trip() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("authorization", "Bearer sk-test")
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "Hello!"}]
        }))
        .send()
        .await
        .expect("proxy request should succeed");

    assert_eq!(resp.status(), 200);

    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["model"], "gpt-4o");
    assert_eq!(body["choices"][0]["message"]["content"], "Hello from mock!");
    assert_eq!(body["usage"]["prompt_tokens"], 10);

    // Give span submission a moment to complete (async spawn).
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1, "exactly one span should be captured");
    let span = &spans[0];
    assert_eq!(span.name, "llm.openai.chat");
    assert_eq!(span.project_id, "integration-test");
    assert!(span.gen_ai.is_some());
    let gen_ai = span.gen_ai.as_ref().unwrap();
    assert_eq!(gen_ai.model, "gpt-4o");
    assert_eq!(gen_ai.input_tokens, 10);
    assert_eq!(gen_ai.output_tokens, 5);
    assert_eq!(gen_ai.total_tokens, 15);
}

/// Test: Unknown provider returns 502.
#[tokio::test]
async fn proxy_unknown_provider_returns_502() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!(
            "{}/proxy/fakeprovider/v1/chat/completions",
            proxy_url
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({"model": "x", "messages": []}))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 502);
    let body = resp.text().await.unwrap();
    assert!(
        body.contains("unknown provider"),
        "error body should mention unknown provider"
    );
}

/// Test: Upstream 5xx error → span with error status + 500 forwarded.
#[tokio::test]
async fn proxy_upstream_error_creates_error_span() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/error", proxy_url))
        .header("content-type", "application/json")
        .json(&serde_json::json!({"model": "gpt-4o", "messages": []}))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 500);

    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(spans[0].status, candela_core::SpanStatus::Error);
    assert!(spans[0].status_message.as_ref().unwrap().contains("500"));
}

/// Test: Baggage header propagates tenant_id into span attributes.
#[tokio::test]
async fn proxy_baggage_tenant_propagation() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("baggage", "candela.tenant_id=acme-corp,env=prod")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "tenant test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);

    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(
        spans[0].attributes.get("candela.tenant_id"),
        Some(&"acme-corp".to_string()),
        "tenant_id from baggage must appear in span attributes"
    );
}

/// INT-5: W3C Traceparent header propagates trace_id + parent_span_id into span.
#[tokio::test]
async fn proxy_traceparent_propagation() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header(
            "traceparent",
            "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
        )
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "trace test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(spans[0].trace_id, "4bf92f3577b34da6a3ce929d0e0e4736");
    assert_eq!(
        spans[0].parent_span_id,
        Some("00f067aa0ba902b7".to_string())
    );
}

/// INT-6: x-request-id response header is returned to client.
#[tokio::test]
async fn proxy_returns_request_id_header() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("x-request-id", "my-custom-req-123")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "req id test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    let req_id = resp
        .headers()
        .get("x-request-id")
        .unwrap()
        .to_str()
        .unwrap();
    assert_eq!(req_id, "my-custom-req-123");
}

/// INT-7: Invalid x-request-id is replaced with a generated ID.
#[tokio::test]
async fn proxy_replaces_invalid_request_id() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("x-request-id", "evil;injection!!")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "injection test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    let req_id = resp
        .headers()
        .get("x-request-id")
        .unwrap()
        .to_str()
        .unwrap();
    // Should be a 32-char hex string (generated), NOT the injected value
    assert_eq!(req_id.len(), 32);
    assert!(req_id.chars().all(|c| c.is_ascii_hexdigit()));
}

/// INT-8: job_id from baggage appears in span.
#[tokio::test]
async fn proxy_baggage_job_id_propagation() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("baggage", "candela.job_id=experiment-42")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "job test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(spans[0].job_id, Some("experiment-42".to_string()));
}

/// INT-9: Cache tokens extracted from OpenAI response into span.
#[tokio::test]
async fn proxy_cache_tokens_openai() {
    // Start a mock that returns cache token data
    let app = Router::new().route(
        "/v1/chat/completions",
        post(|| async {
            Json(serde_json::json!({
                "model": "gpt-4o",
                "choices": [{"message": {"content": "cached!"}, "finish_reason": "stop"}],
                "usage": {
                    "prompt_tokens": 100,
                    "completion_tokens": 20,
                    "total_tokens": 120,
                    "prompt_tokens_details": {"cached_tokens": 80}
                }
            }))
        }),
    );
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let upstream_url = format!("http://127.0.0.1:{}", listener.local_addr().unwrap().port());
    tokio::spawn(async move { axum::serve(listener, app).await.unwrap() });

    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .json(&serde_json::json!({"model": "gpt-4o", "messages": [{"role": "user", "content": "cache test"}]}))
        .send()
        .await
        .unwrap();

    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    let gen_ai = spans[0].gen_ai.as_ref().unwrap();
    assert_eq!(gen_ai.cache_read_tokens, 80);
    assert_eq!(gen_ai.input_tokens, 100);
}

/// INT-10: Session ID with injection chars is silently dropped.
#[tokio::test]
async fn proxy_session_id_validation() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("x-session-id", "evil!!session@id")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "session test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(
        spans[0].session_id, None,
        "invalid session ID must be silently dropped"
    );
}

/// INT-11: x-environment and x-service-name headers appear in span.
#[tokio::test]
async fn proxy_environment_service_headers() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    let resp = client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("x-environment", "staging")
        .header("x-service-name", "my-agent")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "env test"}]
        }))
        .send()
        .await
        .unwrap();

    assert_eq!(resp.status(), 200);
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert_eq!(spans[0].environment, Some("staging".to_string()));
    assert_eq!(spans[0].service_name, Some("my-agent".to_string()));
}

/// INT-12: Span duration is non-zero and reasonable.
#[tokio::test]
async fn proxy_span_duration_nonzero() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "duration test"}]
        }))
        .send()
        .await
        .unwrap();

    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    assert!(
        spans[0].duration.as_micros() > 0,
        "span duration must be >0"
    );
    assert!(
        spans[0].duration.as_secs() < 10,
        "span duration should be <10s for a local mock"
    );
}

/// INT-13: Traceparent takes precedence over x-trace-id header.
#[tokio::test]
async fn proxy_traceparent_overrides_x_trace_id() {
    let (upstream_url, _upstream_handle) = start_mock_upstream().await;
    let submitter = Arc::new(TestSubmitter::new());
    let (proxy_url, _proxy_handle) = start_proxy(&upstream_url, submitter.clone()).await;

    let client = reqwest::Client::new();
    client
        .post(format!("{}/proxy/openai/v1/chat/completions", proxy_url))
        .header("content-type", "application/json")
        .header("x-trace-id", "aaaa0000bbbb1111cccc2222dddd3333")
        .header(
            "traceparent",
            "00-1111222233334444aaaabbbbccccdddd-aabbccddee001122-01",
        )
        .json(&serde_json::json!({
            "model": "gpt-4o",
            "messages": [{"role": "user", "content": "precedence test"}]
        }))
        .send()
        .await
        .unwrap();

    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    let spans = submitter.captured_spans();
    assert_eq!(spans.len(), 1);
    // Traceparent trace_id should win over x-trace-id
    assert_eq!(spans[0].trace_id, "1111222233334444aaaabbbbccccdddd");
}
