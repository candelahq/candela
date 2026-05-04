//! LLM reverse proxy engine with format translation and circuit breaking.
//!
//! This crate implements the core proxy logic that captures LLM API requests,
//! forwards them to upstream providers, and generates observability spans.
//!
//! Ported from: `pkg/proxy/`

pub mod circuit;
pub mod ids;
pub mod parsers;
pub mod translate;

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use candela_core::Span;
use tokio::sync::RwLock;
use tracing::{info, warn};

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
    breakers: RwLock<HashMap<String, circuit::CircuitBreaker>>,
}

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
}
