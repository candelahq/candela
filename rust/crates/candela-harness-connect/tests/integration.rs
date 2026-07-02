//! Integration tests for the ConnectRPC harness server.
//!
//! These tests spin up the full Axum+ConnectRPC stack in-process using an
//! ephemeral port and in-memory storage, then exercise the RPC surface via
//! HTTP using the Connect protocol (application/json).

use std::sync::{Arc, Mutex};

use candela_core::harness::HarnessConfig;
use candela_harness_chat::ChatRuntime;
use candela_harness_storage::{Database, SearchIndex};

/// Start the ConnectRPC server on an OS-assigned port, returning the base URL.
async fn start_test_server() -> String {
    let db = Database::open_in_memory().expect("open in-memory db");
    let db = Arc::new(Mutex::new(db));

    let search = SearchIndex::open_in_memory().expect("open in-memory search");
    let search = Arc::new(Mutex::new(search));

    let config = HarnessConfig {
        model: "test-model".to_string(),
        ..Default::default()
    };
    let chat = Arc::new(ChatRuntime::new(config, db.clone(), search.clone()));

    // Bind to port 0 → OS picks an available port.
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind ephemeral port");
    let port = listener.local_addr().unwrap().port();

    // Build the server exactly like the production path but with our listener.
    let connect_router = candela_harness_connect::server::build_router(chat, db, search);

    use tower_http::cors::CorsLayer;
    use tower_http::trace::TraceLayer;

    let app = axum::Router::new()
        .merge(connect_router.into_axum_router())
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http());

    tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    format!("http://127.0.0.1:{port}")
}

/// POST a Connect unary RPC and return the response body.
async fn connect_post(base: &str, method: &str, body: &str) -> reqwest::Response {
    reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .expect("build client")
        .post(format!("{base}/candela.v1.HarnessService/{method}"))
        .header("Content-Type", "application/json")
        .body(body.to_string())
        .send()
        .await
        .expect("HTTP request failed")
}

#[tokio::test]
async fn test_health() {
    let base = start_test_server().await;
    let resp = connect_post(&base, "Health", "{}").await;
    assert_eq!(resp.status(), 200);
    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["status"], "ok");
}

#[tokio::test]
async fn test_create_and_get_session() {
    let base = start_test_server().await;

    // Create
    let resp = connect_post(&base, "CreateSession", r#"{"model":"test-model"}"#).await;
    assert_eq!(resp.status(), 200);
    let body: serde_json::Value = resp.json().await.unwrap();
    let session = &body["session"];
    let session_id = session["id"].as_str().expect("session id");
    assert!(!session_id.is_empty());
    assert_eq!(session["model"], "test-model");

    // Get
    let resp = connect_post(
        &base,
        "GetSession",
        &format!(r#"{{"session_id":"{session_id}"}}"#),
    )
    .await;
    assert_eq!(resp.status(), 200);
    let got: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(got["session"]["id"], session_id);
}

#[tokio::test]
async fn test_list_sessions_pagination() {
    let base = start_test_server().await;

    // Create 3 sessions
    for _ in 0..3 {
        connect_post(&base, "CreateSession", r#"{"model":"m"}"#).await;
    }

    // List with limit
    let resp = connect_post(&base, "ListSessions", r#"{"limit":2,"offset":0}"#).await;
    assert_eq!(resp.status(), 200);
    let body: serde_json::Value = resp.json().await.unwrap();
    let sessions = body["sessions"].as_array().expect("sessions array");
    assert_eq!(sessions.len(), 2);
}

#[tokio::test]
async fn test_delete_session() {
    let base = start_test_server().await;

    // Create
    let resp = connect_post(&base, "CreateSession", r#"{"model":"m"}"#).await;
    let body: serde_json::Value = resp.json().await.unwrap();
    let id = body["session"]["id"].as_str().unwrap();

    // Delete
    let resp = connect_post(
        &base,
        "DeleteSession",
        &format!(r#"{{"session_id":"{id}"}}"#),
    )
    .await;
    assert_eq!(resp.status(), 200);

    // Get should 404
    let resp = connect_post(&base, "GetSession", &format!(r#"{{"session_id":"{id}"}}"#)).await;
    // Connect protocol returns HTTP 200 with error in body, or non-200
    // depending on implementation. Check for error.
    let body = resp.text().await.unwrap();
    assert!(
        body.contains("not_found") || body.contains("not found") || body.contains("NOT_FOUND"),
        "expected not_found error, got: {body}"
    );
}

#[tokio::test]
async fn test_edit_session_title() {
    let base = start_test_server().await;

    // Create
    let resp = connect_post(&base, "CreateSession", r#"{"model":"m"}"#).await;
    let body: serde_json::Value = resp.json().await.unwrap();
    let id = body["session"]["id"].as_str().unwrap();

    // Edit title
    let resp = connect_post(
        &base,
        "EditSessionTitle",
        &format!(r#"{{"session_id":"{id}","title":"Updated Title"}}"#),
    )
    .await;
    assert_eq!(resp.status(), 200);
    let updated: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(updated["session"]["title"], "Updated Title");
}

#[tokio::test]
async fn test_get_nonexistent_session_returns_not_found() {
    let base = start_test_server().await;

    let resp = connect_post(&base, "GetSession", r#"{"session_id":"does-not-exist"}"#).await;
    let body = resp.text().await.unwrap();
    assert!(
        body.contains("not_found") || body.contains("not found") || body.contains("NOT_FOUND"),
        "expected not_found, got: {body}"
    );
}

#[tokio::test]
async fn test_send_message_streams_error_without_proxy() {
    let base = start_test_server().await;

    // Create a session first
    let resp = connect_post(&base, "CreateSession", r#"{"model":"m"}"#).await;
    let body: serde_json::Value = resp.json().await.unwrap();
    let id = body["session"]["id"].as_str().unwrap();

    // SendMessage — no proxy URL configured, so the LLM call will fail.
    // We should get a stream that contains an error event.
    let resp = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .expect("build client")
        .post(format!("{base}/candela.v1.HarnessService/SendMessage"))
        .header("Content-Type", "application/connect+json")
        .body(format!(r#"{{"session_id":"{id}","content":"hello"}}"#))
        .send()
        .await
        .expect("streaming request");

    // For server-streaming Connect, the response body contains newline-delimited
    // JSON envelopes. We should get at least one message (the error event).
    let body = resp.text().await.unwrap();
    assert!(
        !body.is_empty(),
        "expected streaming response body, got empty"
    );
    // Verify the error event is present — the stream should contain an error
    // since no LLM proxy is configured. This validates the full plumbing:
    // request → spawn → callback → channel → stream → response.
    assert!(
        body.contains("error") || body.contains("Error"),
        "expected error event in streaming response, got: {body}"
    );
}
