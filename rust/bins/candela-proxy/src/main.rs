//! Candela Proxy — standalone LLM observability proxy.
//!
//! A single-binary reverse proxy that captures LLM API calls, calculates costs,
//! and exports observability spans. Supports both environment variables and an
//! optional YAML config file for flexible deployment across containers, VMs,
//! and developer machines.
//!
//! # Configuration
//!
//! Environment variables take precedence over config file values.
//!
//!   PORT                — HTTP listen port (default: 8080)
//!   CONFIG_FILE         — Path to YAML config file (optional)
//!   GCP_PROJECT         — GCP project for Vertex AI + Pub/Sub
//!   VERTEX_REGION       — Vertex AI region (default: us-central1)
//!   PROVIDERS           — comma-separated provider list (default: openai,anthropic)
//!   CANDELA_PROJECT_ID  — project ID for span tagging
//!   PUBSUB_TOPIC        — Pub/Sub topic for span export (optional)
//!   OTLP_ENDPOINT       — OTLP/HTTP endpoint for span export (optional)
//!   OTLP_HEADERS        — comma-separated key=value OTLP auth headers
//!   CORS_ORIGINS        — comma-separated allowed origins (default: *)
//!   LOG_FORMAT          — "json" (default) or "text" for human-readable logs
//!
//! # Usage
//!
//!   candela-proxy                    # Start with env-var config
//!   candela-proxy version            # Print version and exit
//!   CONFIG_FILE=config.yaml candela-proxy  # Start with config file

use std::env;
use std::sync::Arc;

use axum::{Router, routing::get};
use tokio::net::TcpListener;
use tower_http::cors::{AllowOrigin, CorsLayer};
use tracing::info;

use candela_core::Span;
use candela_proxy::handler::{self, AppState};
use candela_proxy::{Config, Provider, SpanSubmitter};

/// Version string — injected at build time via `--cfg` or defaults to dev.
const VERSION: &str = env!("CARGO_PKG_VERSION");

/// Log-and-forward span submitter.
///
/// In production this would be backed by Pub/Sub or OTLP; the standalone
/// proxy uses a log submitter by default and will gain sink configuration
/// in a follow-up.
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
                cost_usd = span.gen_ai.as_ref().map(|g| g.cost_usd).unwrap_or(0.0),
                "span captured"
            );
        }
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Handle `version` subcommand.
    if env::args().nth(1).as_deref() == Some("version") {
        println!("candela-proxy {VERSION}");
        return Ok(());
    }

    // Structured logging — JSON for containers, text for humans.
    let log_format = env_or("LOG_FORMAT", "json");
    if log_format == "text" {
        tracing_subscriber::fmt()
            .with_env_filter(
                tracing_subscriber::EnvFilter::from_default_env()
                    .add_directive(tracing::Level::INFO.into()),
            )
            .init();
    } else {
        tracing_subscriber::fmt()
            .json()
            .with_env_filter(
                tracing_subscriber::EnvFilter::from_default_env()
                    .add_directive(tracing::Level::INFO.into()),
            )
            .init();
    }

    // ── Configuration ──
    let port = env_or("PORT", "8080");
    let gcp_project = env::var("GCP_PROJECT").unwrap_or_default();
    let project_id = env_or("CANDELA_PROJECT_ID", &gcp_project);

    if project_id.is_empty() {
        tracing::warn!(
            "CANDELA_PROJECT_ID and GCP_PROJECT are both unset — spans will have an empty project_id"
        );
    }

    // ── Build providers from env ──
    let providers_csv = env_or("PROVIDERS", "openai,anthropic");
    let vertex_region = env_or("VERTEX_REGION", "us-central1");
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
                "anthropic-direct" => "https://api.anthropic.com".to_string(),
                "google" => {
                    if gcp_project.is_empty() {
                        tracing::warn!(provider = name, "GCP_PROJECT is required for google provider — skipping");
                        return None;
                    }
                    format!(
                        "https://{}-aiplatform.googleapis.com/v1/projects/{}/locations/{}/publishers/google/models",
                        vertex_region, gcp_project, vertex_region
                    )
                }
                _ => {
                    // Check for env-provided upstream: {NAME}_UPSTREAM_URL
                    let env_key = format!("{}_UPSTREAM_URL", name.to_uppercase().replace('-', "_"));
                    match env::var(&env_key) {
                        Ok(url) => url,
                        Err(_) => {
                            tracing::warn!(provider = name, env_key = %env_key, "no upstream URL — skipping");
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
                mitm: None,
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
    let cors_layer = build_cors_layer(&cors_origins);

    // ── Build proxy ──
    let proxy = Arc::new(candela_proxy::Proxy::new(Config {
        providers,
        project_id: project_id.clone(),
    }));

    let app_state = Arc::new(AppState {
        proxy,
        submitter: Arc::new(LogSubmitter),
        http_client: reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(300)) // 5min — matches Go proxy
            .build()
            .expect("failed to build HTTP client"),
    });

    info!(
        port = %port,
        project_id = %project_id,
        cors_origins = %cors_origins,
        version = VERSION,
        "🕯️ candela-proxy starting"
    );

    // ── HTTP server ──
    let app = Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .route("/version", get(version))
        .route(
            "/proxy/{provider}/{*rest}",
            axum::routing::any(handler::proxy_handler),
        )
        .layer(cors_layer)
        .with_state(app_state);

    let addr = format!("0.0.0.0:{port}");
    let listener = TcpListener::bind(&addr).await?;
    info!(addr = %addr, "🕯️ candela-proxy listening");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    info!("candela-proxy stopped");
    Ok(())
}

// ── Handlers ──

async fn healthz() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "ok",
        "binary": "candela-proxy",
        "version": VERSION,
    }))
}

async fn readyz() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({"status": "ready"}))
}

async fn version() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({"version": VERSION}))
}

// ── Helpers ──

fn build_cors_layer(cors_origins: &str) -> CorsLayer {
    if cors_origins == "*" {
        return CorsLayer::permissive();
    }

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

    #[test]
    fn env_or_returns_default_when_unset() {
        let result = env_or("CANDELA_TEST_NONEXISTENT_VAR_PROXY_12345", "fallback");
        assert_eq!(result, "fallback");
    }

    #[test]
    fn env_or_returns_env_when_set() {
        let result = env_or("PATH", "fallback");
        assert_ne!(result, "fallback");
        assert!(!result.is_empty());
    }

    #[test]
    fn build_cors_permissive_on_wildcard() {
        // Should not panic.
        let _layer = build_cors_layer("*");
    }

    #[test]
    fn build_cors_permissive_on_empty() {
        // Empty origins should fall back to permissive without panic.
        let _layer = build_cors_layer("");
    }

    #[test]
    fn build_cors_specific_origins() {
        let _layer = build_cors_layer("http://localhost:3000,http://localhost:8181");
    }

    #[test]
    fn version_string_not_empty() {
        assert!(!VERSION.is_empty());
    }
}
