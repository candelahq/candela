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

    /// Upstream hostname for SNI matching in transparent proxy mode.
    /// If `None`, derived automatically from `upstream_url`.
    pub host: Option<String>,

    /// Wildcard pattern for SNI matching (e.g. `"*.aiplatform.googleapis.com"`).
    /// When set, takes precedence over `host` for wildcard matching.
    pub host_pattern: Option<String>,

    /// Whether the transparent proxy listener intercepts connections to this
    /// provider's host. Providers sharing a host with another (already intercepted)
    /// provider should set this to `false`. Default: `true` (`None` = true).
    pub intercept: Option<bool>,

    pub format_translator: Option<Arc<dyn FormatTranslator>>,
    pub path_rewriter: Option<Arc<dyn PathRewriter>>,
    // TODO: token_source for ADC injection
}

impl Provider {
    /// Returns the hostname used for SNI matching.
    /// Returns `host` if explicitly set, otherwise parses hostname from `upstream_url`.
    pub fn effective_host(&self) -> Option<String> {
        if let Some(ref h) = self.host {
            return Some(h.clone());
        }
        // Parse hostname from upstream_url.
        url::Url::parse(&self.upstream_url)
            .ok()
            .and_then(|u| u.host_str().map(|s| s.to_string()))
    }

    /// Returns whether the transparent proxy should intercept connections to this
    /// provider's host. Defaults to `true` if `intercept` is `None`.
    pub fn should_intercept(&self) -> bool {
        self.intercept.unwrap_or(true)
    }
}

/// Maps TLS SNI hostnames to their corresponding provider names.
/// Used by the transparent proxy listener to route intercepted connections.
pub struct SNIMap {
    /// Exact hostname → provider name (e.g. "api.openai.com" → "openai").
    exact: HashMap<String, String>,
    /// Wildcard suffix → provider name (stored without "*." prefix).
    /// E.g. "aiplatform.googleapis.com" → "anthropic".
    wildcards: HashMap<String, String>,
}

impl SNIMap {
    /// Creates an SNI hostname→provider lookup from a list of providers.
    /// Only providers where `should_intercept()` returns true are included.
    /// Duplicate hostnames are deduplicated (first provider wins).
    pub fn build(providers: &[Provider]) -> Self {
        let mut exact = HashMap::new();
        let mut wildcards = HashMap::new();

        for p in providers {
            if !p.should_intercept() {
                continue;
            }
            // Register wildcard pattern if present.
            if let Some(ref pattern) = p.host_pattern {
                // Store the suffix after "*" (e.g. "*.foo.com" → ".foo.com",
                // "*-foo.com" → "-foo.com").
                let suffix = pattern.strip_prefix('*').unwrap_or(pattern);
                wildcards
                    .entry(suffix.to_string())
                    .or_insert_with(|| p.name.clone());
                continue;
            }
            // Register exact hostname.
            if let Some(host) = p.effective_host() {
                exact.entry(host).or_insert_with(|| p.name.clone());
            }
        }

        Self { exact, wildcards }
    }

    /// Returns the provider name for the given SNI hostname.
    /// Checks exact matches first, then wildcard suffix matches.
    pub fn lookup(&self, hostname: &str) -> Option<&str> {
        // Exact match.
        if let Some(name) = self.exact.get(hostname) {
            return Some(name.as_str());
        }
        // Wildcard suffix match: e.g. "us-central1-aiplatform.googleapis.com"
        // matches "*-aiplatform.googleapis.com" (suffix "-aiplatform.googleapis.com").
        // Also supports subdomain wildcards: "sub.example.com" matches
        // "*.example.com" (suffix ".example.com").
        for (suffix, name) in &self.wildcards {
            if hostname.len() > suffix.len() && hostname[hostname.len() - suffix.len()..] == *suffix
            {
                return Some(name.as_str());
            }
        }
        None
    }

    /// Returns all unique hostnames and patterns registered in the map.
    /// Useful for generating enforcement resources (FQDNNetworkPolicy, Tetragon).
    pub fn hosts(&self) -> Vec<String> {
        let mut result: Vec<String> = self.exact.keys().cloned().collect();
        for suffix in self.wildcards.keys() {
            result.push(format!("*{}", suffix));
        }
        result
    }
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

    /// Helper to create a minimal provider for testing.
    fn test_provider(name: &str, upstream: &str) -> Provider {
        Provider {
            name: name.into(),
            upstream_url: upstream.into(),
            host: None,
            host_pattern: None,
            intercept: None,
            format_translator: None,
            path_rewriter: None,
        }
    }

    #[test]
    fn proxy_new_registers_providers() {
        let config = Config {
            providers: vec![
                test_provider("openai", "https://api.openai.com"),
                test_provider("anthropic", "https://api.anthropic.com"),
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
            providers: vec![test_provider("test", "http://localhost")],
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

    // ── Provider host/intercept tests ──

    #[test]
    fn effective_host_from_url() {
        let p = test_provider("openai", "https://api.openai.com");
        assert_eq!(p.effective_host().as_deref(), Some("api.openai.com"));
    }

    #[test]
    fn effective_host_from_url_with_path() {
        let p = test_provider(
            "gemini",
            "https://generativelanguage.googleapis.com/v1beta/openai",
        );
        assert_eq!(
            p.effective_host().as_deref(),
            Some("generativelanguage.googleapis.com")
        );
    }

    #[test]
    fn effective_host_explicit_overrides_url() {
        let p = Provider {
            host: Some("custom.example.com".into()),
            ..test_provider("custom", "https://api.openai.com")
        };
        assert_eq!(p.effective_host().as_deref(), Some("custom.example.com"));
    }

    #[test]
    fn effective_host_empty_url() {
        let p = test_provider("empty", "");
        assert_eq!(p.effective_host(), None);
    }

    #[test]
    fn should_intercept_default_true() {
        let p = test_provider("openai", "https://api.openai.com");
        assert!(p.should_intercept());
    }

    #[test]
    fn should_intercept_explicit_false() {
        let p = Provider {
            intercept: Some(false),
            ..test_provider("gemini-oai", "https://example.com")
        };
        assert!(!p.should_intercept());
    }

    // ── SNIMap tests ──

    #[test]
    fn sni_map_exact_lookup() {
        let providers = vec![
            test_provider("openai", "https://api.openai.com"),
            test_provider("anthropic-direct", "https://api.anthropic.com"),
        ];
        let m = SNIMap::build(&providers);

        assert_eq!(m.lookup("api.openai.com"), Some("openai"));
        assert_eq!(m.lookup("api.anthropic.com"), Some("anthropic-direct"));
        assert_eq!(m.lookup("unknown.com"), None);
    }

    #[test]
    fn sni_map_wildcard_lookup() {
        let providers = vec![Provider {
            host_pattern: Some("*-aiplatform.googleapis.com".into()),
            ..test_provider("anthropic", "https://us-central1-aiplatform.googleapis.com")
        }];
        let m = SNIMap::build(&providers);

        assert_eq!(
            m.lookup("us-central1-aiplatform.googleapis.com"),
            Some("anthropic")
        );
        assert_eq!(
            m.lookup("europe-west4-aiplatform.googleapis.com"),
            Some("anthropic")
        );
        assert_eq!(m.lookup("api.openai.com"), None);
    }

    #[test]
    fn sni_map_excludes_non_intercepted() {
        let providers = vec![
            test_provider("google", "https://generativelanguage.googleapis.com"),
            Provider {
                intercept: Some(false),
                ..test_provider(
                    "gemini-oai",
                    "https://generativelanguage.googleapis.com/v1beta/openai",
                )
            },
        ];
        let m = SNIMap::build(&providers);

        // Should resolve to "google", not "gemini-oai".
        assert_eq!(
            m.lookup("generativelanguage.googleapis.com"),
            Some("google")
        );
    }

    #[test]
    fn sni_map_hosts_returns_all() {
        let providers = vec![
            test_provider("openai", "https://api.openai.com"),
            test_provider("anthropic-direct", "https://api.anthropic.com"),
            Provider {
                host_pattern: Some("*-aiplatform.googleapis.com".into()),
                ..test_provider("anthropic", "https://us-central1-aiplatform.googleapis.com")
            },
        ];
        let m = SNIMap::build(&providers);
        let hosts = m.hosts();

        // 2 exact + 1 wildcard = 3
        assert_eq!(hosts.len(), 3);
        assert!(hosts.contains(&"api.openai.com".to_string()));
        assert!(hosts.contains(&"api.anthropic.com".to_string()));
        assert!(hosts.contains(&"*-aiplatform.googleapis.com".to_string()));
    }

    #[test]
    fn sni_map_first_provider_wins_dedup() {
        let providers = vec![
            test_provider("first", "https://api.openai.com"),
            test_provider("second", "https://api.openai.com"),
        ];
        let m = SNIMap::build(&providers);
        assert_eq!(m.lookup("api.openai.com"), Some("first"));
    }

    /// SECURITY: verify both suffix patterns (*-foo.com) and subdomain
    /// patterns (*.foo.com) work correctly.
    #[test]
    fn sni_map_wildcard_boundary_security() {
        // Test suffix pattern (GCP Vertex AI style).
        let providers = vec![Provider {
            host_pattern: Some("*-aiplatform.googleapis.com".into()),
            ..test_provider("anthropic", "https://us-central1-aiplatform.googleapis.com")
        }];
        let m = SNIMap::build(&providers);

        // Should match: GCP regional endpoints.
        assert_eq!(
            m.lookup("us-central1-aiplatform.googleapis.com"),
            Some("anthropic"),
            "valid GCP region should match"
        );
        assert_eq!(
            m.lookup("europe-west4-aiplatform.googleapis.com"),
            Some("anthropic"),
            "valid GCP region should match"
        );

        // Must NOT match: bare suffix (no prefix).
        assert_eq!(
            m.lookup("aiplatform.googleapis.com"),
            None,
            "bare suffix must not match"
        );

        // Test subdomain pattern (traditional wildcard).
        let sub_providers = vec![Provider {
            host_pattern: Some("*.example.com".into()),
            ..test_provider("test", "https://example.com")
        }];
        let sm = SNIMap::build(&sub_providers);

        assert_eq!(
            sm.lookup("sub.example.com"),
            Some("test"),
            "subdomain should match"
        );
        assert_eq!(
            sm.lookup("deep.sub.example.com"),
            Some("test"),
            "deep subdomain should match"
        );
        assert_eq!(sm.lookup("example.com"), None, "bare domain must not match");
        assert_eq!(
            sm.lookup("notexample.com"),
            None,
            "different domain must not match"
        );
    }
}
