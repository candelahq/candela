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
use tower_http::cors::{AllowOrigin, CorsLayer};
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

    // ── CORS configuration ──
    // Parse CORS_ORIGINS env var into allowed origins.
    // Default "*" is permissive; production should set explicit origins.
    let cors_origins = env_or("CORS_ORIGINS", "*");
    let cors_layer = if cors_origins == "*" {
        CorsLayer::permissive()
    } else {
        let origins: Vec<_> = cors_origins
            .split(',')
            .filter_map(|s| {
                let trimmed = s.trim();
                if trimmed.is_empty() {
                    None
                } else {
                    trimmed.parse().ok()
                }
            })
            .collect();
        if origins.is_empty() {
            tracing::warn!("CORS_ORIGINS contained no valid origins, falling back to permissive");
            CorsLayer::permissive()
        } else {
            CorsLayer::new()
                .allow_origin(AllowOrigin::list(origins))
                .allow_methods(tower_http::cors::Any)
                .allow_headers(tower_http::cors::Any)
        }
    };

    info!(
        port = %port,
        gcp_project = %_gcp_project,
        vertex_region = %_vertex_region,
        project_id = %_project_id,
        cors_origins = %cors_origins,
        "🕯️ candela-sidecar starting"
    );

    // TODO: Initialize span writers (#125)
    // TODO: Initialize span processor (#124)
    // TODO: Initialize LLM proxy (#123)

    // ── HTTP server ──
    let app = Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .layer(cors_layer);

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
    match tokio::signal::ctrl_c().await {
        Ok(()) => info!("shutting down..."),
        Err(e) => tracing::error!(error = %e, "failed to listen for ctrl+c, shutting down anyway"),
    }
}

fn env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    // U-15: env_or returns default when env var is unset.
    #[test]
    fn env_or_returns_default_when_unset() {
        // Use a key that definitely doesn't exist.
        let result = env_or("CANDELA_TEST_NONEXISTENT_VAR_12345", "fallback");
        assert_eq!(result, "fallback");
    }

    #[test]
    fn env_or_returns_env_when_set() {
        // Test env_or logic without mutating global state:
        // env::var succeeds → env_or returns its value (not the default).
        // We rely on PATH always being set on all platforms.
        let result = env_or("PATH", "fallback");
        assert_ne!(result, "fallback");
        assert!(!result.is_empty());
    }
}
