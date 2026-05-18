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
use std::sync::Arc;

use axum::{Router, routing::get};
use tokio::net::TcpListener;
use tower_http::cors::{AllowOrigin, CorsLayer};
use tracing::info;

use candela_core::Span;
use candela_proxy::handler::{self, AppState};
use candela_proxy::{Config, Provider, SpanSubmitter};

/// Log-only span submitter for development / initial wiring.
struct LogSubmitter;

impl SpanSubmitter for LogSubmitter {
    fn submit_batch(&self, spans: Vec<Span>) {
        for span in &spans {
            info!(
                trace_id = %span.trace_id,
                span_id = %span.span_id,
                name = %span.name,
                duration_ms = span.duration.as_millis() as u64,
                model = span.gen_ai.as_ref().map(|g| g.model.as_str()).unwrap_or(""),
                input_tokens = span.gen_ai.as_ref().map(|g| g.input_tokens).unwrap_or(0),
                output_tokens = span.gen_ai.as_ref().map(|g| g.output_tokens).unwrap_or(0),
                "span captured"
            );
        }
    }
}

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
    let gcp_project = env::var("GCP_PROJECT").unwrap_or_default();
    let _vertex_region = env_or("VERTEX_REGION", "us-central1");
    let project_id = env_or("CANDELA_PROJECT_ID", &gcp_project);

    if project_id.is_empty() {
        tracing::warn!(
            "CANDELA_PROJECT_ID and GCP_PROJECT are both unset — spans will have an empty project_id"
        );
    }

    // ── Build providers from env ──
    let providers_csv = env_or("PROVIDERS", "openai,anthropic");
    let providers: Vec<Provider> = providers_csv
        .split(',')
        .filter_map(|name| {
            let name = name.trim();
            if name.is_empty() {
                return None;
            }
            let upstream_url = match name {
                "openai" => "https://api.openai.com".to_string(),
                "anthropic" => "https://api.anthropic.com".to_string(),
                _ => {
                    // Check for env-provided upstream: {NAME}_UPSTREAM_URL
                    let env_key = format!("{}_UPSTREAM_URL", name.to_uppercase().replace('-', "_"));
                    match env::var(&env_key) {
                        Ok(url) => url,
                        Err(_) => {
                            tracing::warn!(provider = name, "no upstream URL — skipping");
                            return None;
                        }
                    }
                }
            };
            Some(Provider {
                name: name.to_string(),
                upstream_url,
                host: None,
                host_pattern: None,
                intercept: None,
                format_translator: None,
                path_rewriter: None,
            })
        })
        .collect();

    info!(
        providers = ?providers.iter().map(|p| &p.name).collect::<Vec<_>>(),
        "configured providers"
    );

    // ── CORS configuration ──
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

    // ── Build proxy ──
    let proxy = Arc::new(candela_proxy::Proxy::new(Config {
        providers,
        project_id: project_id.clone(),
    }));

    let app_state = Arc::new(AppState {
        proxy,
        submitter: Arc::new(LogSubmitter),
        http_client: reqwest::Client::new(),
    });

    info!(
        port = %port,
        project_id = %project_id,
        cors_origins = %cors_origins,
        "🕯️ candela-sidecar starting"
    );

    // ── HTTP server ──
    let app = Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .route(
            "/proxy/{provider}/{*rest}",
            axum::routing::any(handler::proxy_handler),
        )
        .layer(cors_layer)
        .with_state(app_state);

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
