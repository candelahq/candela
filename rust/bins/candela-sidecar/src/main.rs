//! Candela sidecar — minimal production LLM proxy for container environments.
//!
//! Routes LLM traffic through configurable providers with ADC credential
//! injection and exports observability spans to Pub/Sub and/or OTLP sinks.
//! No Firebase, no local storage, no UI — pure proxy + telemetry.
//!
//! Configuration is entirely via environment variables:
//!
//!   PORT               — HTTP listen port (default: 8080)
//!   GCP_PROJECT        — GCP project for Vertex AI + Pub/Sub
//!   VERTEX_REGION      — Vertex AI region (default: us-central1)
//!   PROVIDERS          — comma-separated provider list (default: all)
//!   CANDELA_PROJECT_ID — project ID for span tagging
//!   PUBSUB_TOPIC       — Pub/Sub topic for span export (optional)
//!   SPAN_FORMAT        — "proto" (default) or "json" for Pub/Sub messages
//!   OTLP_ENDPOINT      — OTLP/HTTP endpoint for span export (optional)
//!   OTLP_HEADERS       — comma-separated key=value OTLP auth headers
//!   CORS_ORIGINS       — comma-separated allowed origins (default: *)
//!
//! Ported from: `cmd/candela-sidecar/main.go`

use std::env;

use axum::{Router, routing::get};
use tokio::net::TcpListener;
use tower_http::cors::CorsLayer;
use tracing::info;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Structured JSON logging.
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into()),
        )
        .init();

    // ── Configuration ──
    let port = env_or("PORT", "8080");
    let _gcp_project = env::var("GCP_PROJECT").unwrap_or_default();
    let _vertex_region = env_or("VERTEX_REGION", "us-central1");
    let _project_id = env_or("CANDELA_PROJECT_ID", &_gcp_project);

    if _project_id.is_empty() {
        tracing::warn!(
            "CANDELA_PROJECT_ID and GCP_PROJECT are both unset — spans will have an empty project_id"
        );
    }

    info!(
        port = %port,
        gcp_project = %_gcp_project,
        vertex_region = %_vertex_region,
        project_id = %_project_id,
        "🕯️ candela-sidecar starting"
    );

    // TODO: Initialize span writers (#125)
    // TODO: Initialize span processor (#124)
    // TODO: Initialize LLM proxy (#123)

    // ── HTTP server ──
    let app = Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .layer(CorsLayer::permissive());

    let addr = format!("0.0.0.0:{port}");
    let listener = TcpListener::bind(&addr).await?;
    info!(addr = %addr, "🕯️ candela-sidecar listening");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    info!("sidecar stopped");
    Ok(())
}

async fn healthz() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "ok",
        "binary": "candela-sidecar"
    }))
}

async fn readyz() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({"status": "ready"}))
}

async fn shutdown_signal() {
    tokio::signal::ctrl_c()
        .await
        .expect("failed to listen for ctrl+c");
    info!("shutting down...");
}

fn env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}
