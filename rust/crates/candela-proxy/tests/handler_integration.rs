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
