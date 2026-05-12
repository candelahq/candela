//! LLM reverse proxy engine with format translation and circuit breaking.
//!
//! This crate implements the core proxy logic that captures LLM API requests,
//! forwards them to upstream providers, and generates observability spans.
//!
//! Ported from: `pkg/proxy/`

pub mod attribution;
pub mod circuit;
pub mod handler;
pub mod ids;
pub mod parsers;
pub mod translate;

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use candela_core::Span;
use tokio::sync::RwLock;

/// Handles request/response format translation between client format
/// (e.g. OpenAI Chat Completions) and upstream provider format.
pub trait FormatTranslator: Send + Sync {
    /// Converts the client request body to the upstream format.
    fn translate_request(&self, body: &[u8]) -> anyhow::Result<(Vec<u8>, String)>;

    /// Converts the upstream response body to the client format.
    fn translate_response(&self, body: &[u8], model: &str) -> anyhow::Result<Vec<u8>>;

    /// Converts a single upstream SSE data payload to client format.
    fn translate_stream_chunk(&self, chunk: &[u8], model: &str) -> anyhow::Result<Vec<u8>>;
}

/// Rewrites upstream URL paths for provider-specific routing.
pub trait PathRewriter: Send + Sync {
    /// Returns the upstream URL path for the given model.
    fn rewrite_path(&self, model: &str, streaming: bool) -> String;
}

/// Submits span batches to the processing pipeline.
pub trait SpanSubmitter: Send + Sync {
    fn submit_batch(&self, spans: Vec<Span>);
}

/// LLM API provider configuration.
#[derive(Clone)]
pub struct Provider {
    pub name: String,
    pub upstream_url: String,
    pub format_translator: Option<Arc<dyn FormatTranslator>>,
    pub path_rewriter: Option<Arc<dyn PathRewriter>>,
    // TODO: token_source for ADC injection
}

/// Proxy configuration.
pub struct Config {
    pub providers: Vec<Provider>,
    pub project_id: String,
}

/// LLM API proxy with observability.
///
/// Routes requests to configured providers, captures request/response data,
/// and generates spans for the processing pipeline.
pub struct Proxy {
    providers: HashMap<String, Provider>,
    project_id: String,
    /// Per-provider circuit breakers. The outer RwLock protects the map structure
    /// (only write-locked when adding a new provider). Each breaker has its own
    /// Mutex so concurrent requests to different providers don't contend.
    breakers: RwLock<HashMap<String, tokio::sync::Mutex<circuit::CircuitBreaker>>>,
}

/// Default circuit breaker settings.
const DEFAULT_CB_THRESHOLD: u32 = 5;
const DEFAULT_CB_TIMEOUT: Duration = Duration::from_secs(30);

impl Proxy {
    /// Create a new proxy from configuration.
    pub fn new(config: Config) -> Self {
        let providers = config
            .providers
            .into_iter()
            .map(|p| (p.name.clone(), p))
            .collect();

        Self {
            providers,
            project_id: config.project_id,
            breakers: RwLock::new(HashMap::new()),
        }
    }

    /// Returns the list of active provider names.
    pub fn provider_names(&self) -> Vec<&str> {
        self.providers.keys().map(|s| s.as_str()).collect()
    }

    /// Returns the project ID for span tagging.
    pub fn project_id(&self) -> &str {
        &self.project_id
    }

    /// Look up a provider by name.
    pub fn get_provider(&self, name: &str) -> Option<&Provider> {
        self.providers.get(name)
    }

    /// Check whether the circuit breaker allows a request to the given provider.
    ///
    /// Takes a read lock on the breaker map. Only escalates to a write lock
    /// if the provider's breaker doesn't exist yet (first request to that provider).
    pub async fn check_circuit(&self, provider: &str) -> bool {
        // Fast path: read lock — no contention between different providers.
        {
            let breakers = self.breakers.read().await;
            if let Some(cb) = breakers.get(provider) {
                return cb.lock().await.is_allowed();
            }
        }
        // Slow path: write lock to insert a new breaker (once per provider).
        let mut breakers = self.breakers.write().await;
        let cb = breakers.entry(provider.to_string()).or_insert_with(|| {
            tokio::sync::Mutex::new(circuit::CircuitBreaker::new(
                DEFAULT_CB_THRESHOLD,
                DEFAULT_CB_TIMEOUT,
            ))
        });
        cb.lock().await.is_allowed()
    }

    /// Record a successful upstream call for the given provider.
    pub async fn record_success(&self, provider: &str) {
        let breakers = self.breakers.read().await;
        if let Some(cb) = breakers.get(provider) {
            cb.lock().await.record_success();
        }
    }

    /// Record a failed upstream call for the given provider.
    pub async fn record_failure(&self, provider: &str) {
        // Fast path: read lock.
        {
            let breakers = self.breakers.read().await;
            if let Some(cb) = breakers.get(provider) {
                cb.lock().await.record_failure();
                return;
            }
        }
        // Slow path: insert + record.
        let mut breakers = self.breakers.write().await;
        let cb = breakers.entry(provider.to_string()).or_insert_with(|| {
            tokio::sync::Mutex::new(circuit::CircuitBreaker::new(
                DEFAULT_CB_THRESHOLD,
                DEFAULT_CB_TIMEOUT,
            ))
        });
        cb.lock().await.record_failure();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn proxy_new_registers_providers() {
        let config = Config {
            providers: vec![
                Provider {
                    name: "openai".into(),
                    upstream_url: "https://api.openai.com".into(),
                    format_translator: None,
                    path_rewriter: None,
                },
                Provider {
                    name: "anthropic".into(),
                    upstream_url: "https://api.anthropic.com".into(),
                    format_translator: None,
                    path_rewriter: None,
                },
            ],
            project_id: "test".into(),
        };

        let proxy = Proxy::new(config);
        assert_eq!(proxy.provider_names().len(), 2);
        assert!(proxy.get_provider("openai").is_some());
        assert!(proxy.get_provider("anthropic").is_some());
        assert!(proxy.get_provider("gemini").is_none());
    }

    #[test]
    fn proxy_project_id() {
        let proxy = Proxy::new(Config {
            providers: vec![],
            project_id: "my-project".into(),
        });
        assert_eq!(proxy.project_id(), "my-project");
    }

    #[tokio::test]
    async fn circuit_breaker_integration() {
        let proxy = Proxy::new(Config {
            providers: vec![Provider {
                name: "test".into(),
                upstream_url: "http://localhost".into(),
                format_translator: None,
                path_rewriter: None,
            }],
            project_id: "p".into(),
        });

        // Initially allowed.
        assert!(proxy.check_circuit("test").await);

        // Trip it.
        for _ in 0..DEFAULT_CB_THRESHOLD {
            proxy.record_failure("test").await;
        }
        assert!(!proxy.check_circuit("test").await);

        // Unknown provider should still be allowed (creates fresh breaker).
        assert!(proxy.check_circuit("unknown").await);
    }
}
