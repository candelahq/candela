// Package proxy implements an LLM API proxy that captures requests/responses
// and creates observability spans. Supports OpenAI, Google Gemini, and Anthropic (via Vertex AI).
//
// Usage:
//
//	client = OpenAI(base_url="http://localhost:8080/proxy/openai/v1")
//	client = anthropic.Anthropic(base_url="http://localhost:8080/proxy/anthropic")
//
// The proxy forwards requests transparently, captures the full exchange,
// extracts token usage, calculates cost, and stores as a trace.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"

	"github.com/candelahq/candela/pkg/attribution"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/cloudauth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/notify"
	"github.com/candelahq/candela/pkg/proxy/spendoutbox"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanSubmitter is the interface for submitting spans to the processing pipeline.
type SpanSubmitter interface {
	SubmitBatch(spans []storage.Span)
}

// FormatTranslator handles request/response format translation between the client
// format (e.g. OpenAI Chat Completions) and the upstream provider format.
// A nil FormatTranslator means transparent passthrough.
type FormatTranslator interface {
	// TranslateRequest converts the client request body to the upstream format.
	// Returns the translated body, extracted model name, and any error.
	TranslateRequest(body []byte) (translated []byte, model string, err error)

	// TranslateResponse converts the upstream response body to the client format.
	TranslateResponse(body []byte, model string) ([]byte, error)

	// TranslateStreamChunk converts a single upstream SSE data payload to client format.
	TranslateStreamChunk(chunk []byte, model string) ([]byte, error)
}

// PathRewriter rewrites the upstream URL path for provider-specific routing
// (e.g. Vertex AI's project-scoped model endpoints). A nil PathRewriter
// means the path from the client request is forwarded as-is.
type PathRewriter interface {
	// RewritePath returns the upstream URL path for the given model.
	// streaming indicates whether this is a streaming request.
	RewritePath(model string, streaming bool) string
}

// DefaultVertexAnthropicVersion is the API version required by Vertex AI rawPredict
// for Anthropic models. Override via Provider.AnthropicVersion if Vertex AI
// introduces a newer version.
const DefaultVertexAnthropicVersion = "vertex-2023-10-16"

// maxStreamDuration is the maximum duration for a streaming upstream request,
// even when using a detached context (context.WithoutCancel). This prevents
// unbounded goroutine/connection leaks if upstream goes silent.
const maxStreamDuration = 15 * time.Minute

// Provider defines an LLM API provider configuration.
type Provider struct {
	Name        string `yaml:"name"`     // "openai", "google", "anthropic", "gemini-oai", "anthropic-bedrock"
	UpstreamURL string `yaml:"upstream"` // e.g. "https://api.openai.com"

	// Host is the upstream hostname for SNI matching in transparent proxy mode.
	// If empty, derived automatically from UpstreamURL.
	Host string `yaml:"host,omitempty"`

	// HostPattern is a wildcard pattern for SNI matching (e.g. "*.aiplatform.googleapis.com").
	// When set, takes precedence over Host for wildcard matching.
	HostPattern string `yaml:"host_pattern,omitempty"`

	// Intercept controls whether the transparent proxy listener intercepts connections
	// to this provider's host. Providers sharing a host with another (already intercepted)
	// provider should set this to false to avoid duplicate entries.
	// Default: true (nil pointer = true).
	Intercept *bool `yaml:"intercept,omitempty"`

	// FormatTranslator handles format translation (nil = transparent passthrough).
	FormatTranslator FormatTranslator `yaml:"-"`

	// PathRewriter rewrites upstream URL paths (nil = forward client path).
	PathRewriter PathRewriter `yaml:"-"`

	// TokenSource provides auto-refreshing auth tokens (e.g. GCP ADC). Nil = forward client auth.
	TokenSource oauth2.TokenSource `yaml:"-"`

	// RequestSigner signs outbound requests for providers that use request-level
	// signing rather than Bearer tokens (e.g., AWS SigV4 for Bedrock).
	// When set, takes precedence over TokenSource for auth injection.
	RequestSigner cloudauth.RequestSigner `yaml:"-"`

	// AnthropicVersion overrides the anthropic_version injected into Vertex AI
	// rawPredict bodies. Empty = DefaultVertexAnthropicVersion.
	AnthropicVersion string `yaml:"-"`

	// APIKey is a static API key for authenticating with the upstream provider.
	// Used by custom providers configured via YAML (populated from AuthEnvVar).
	// When set, injected as "Authorization: Bearer <key>" unless AuthHeader
	// is specified, in which case it is injected as "<AuthHeader>: <key>".
	APIKey string `yaml:"-"`

	// AuthHeader overrides the header name used to send APIKey.
	// If empty, defaults to "Authorization" with "Bearer " prefix.
	// Examples: "x-api-key", "X-Auth-Token".
	AuthHeader string `yaml:"-"`
}

// EffectiveHost returns the hostname used for SNI matching.
// Returns Host if explicitly set, otherwise parses hostname from UpstreamURL.
func (p Provider) EffectiveHost() string {
	if p.Host != "" {
		return p.Host
	}
	u, err := url.Parse(p.UpstreamURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// ShouldIntercept returns whether the transparent proxy should intercept
// connections to this provider's host. Defaults to true if Intercept is nil.
func (p Provider) ShouldIntercept() bool {
	if p.Intercept != nil {
		return *p.Intercept
	}
	return true
}

// SNIMap maps TLS SNI hostnames to their corresponding provider names.
// Used by the transparent proxy listener to route intercepted connections.
type SNIMap struct {
	// exact maps exact hostnames (e.g. "api.openai.com") → provider name.
	exact map[string]string
	// wildcards maps wildcard suffixes (e.g. ".aiplatform.googleapis.com" or
	// "-aiplatform.googleapis.com") → provider name.
	// Stored as the literal suffix after the "*" character.
	wildcards map[string]string
}

// BuildSNIMap creates an SNI hostname→provider lookup from a list of providers.
// Only providers where ShouldIntercept() returns true are included.
// Duplicate hostnames are deduplicated (first provider wins).
func BuildSNIMap(providers []Provider) *SNIMap {
	m := &SNIMap{
		exact:     make(map[string]string),
		wildcards: make(map[string]string),
	}
	for _, p := range providers {
		if !p.ShouldIntercept() {
			continue
		}
		// Register wildcard pattern if present.
		if p.HostPattern != "" {
			// Store the suffix after "*" (e.g. "*.foo.com" → ".foo.com",
			// "*-foo.com" → "-foo.com").
			suffix := strings.TrimPrefix(p.HostPattern, "*")
			if _, exists := m.wildcards[suffix]; !exists {
				m.wildcards[suffix] = p.Name
			}
			continue
		}
		// Register exact hostname.
		host := p.EffectiveHost()
		if host == "" {
			continue
		}
		if _, exists := m.exact[host]; !exists {
			m.exact[host] = p.Name
		}
	}
	return m
}

// Lookup returns the provider name for the given SNI hostname.
// Checks exact matches first, then wildcard suffix matches.
// Returns the provider name and true if found, empty string and false otherwise.
func (m *SNIMap) Lookup(hostname string) (provider string, ok bool) {
	if m == nil {
		return "", false
	}
	// Exact match.
	if name, found := m.exact[hostname]; found {
		return name, true
	}
	// Wildcard suffix match: e.g. "us-central1-aiplatform.googleapis.com"
	// matches "*-aiplatform.googleapis.com" (suffix "-aiplatform.googleapis.com").
	// Also supports subdomain wildcards: "sub.example.com" matches
	// "*.example.com" (suffix ".example.com").
	for suffix, name := range m.wildcards {
		if len(hostname) > len(suffix) &&
			hostname[len(hostname)-len(suffix):] == suffix {
			return name, true
		}
	}
	return "", false
}

// Hosts returns all unique hostnames and patterns registered in the map.
// Useful for generating enforcement resources (FQDNNetworkPolicy, Tetragon).
func (m *SNIMap) Hosts() []string {
	if m == nil {
		return nil
	}
	hosts := make([]string, 0, len(m.exact)+len(m.wildcards))
	for h := range m.exact {
		hosts = append(hosts, h)
	}
	for suffix := range m.wildcards {
		hosts = append(hosts, "*"+suffix)
	}
	return hosts
}

// Proxy handles LLM API proxying with observability.
type Proxy struct {
	providers map[string]Provider
	submitter SpanSubmitter
	calc      *costcalc.Calculator
	client    *http.Client
	projectID string
	breakers  map[string]*CircuitBreaker
	spanSem   chan struct{} // bounds concurrent async span-creation goroutines
	asyncSem  chan struct{} // bounds best-effort async goroutines (touch-active, budget notify)

	// Optional dependencies for team-mode features.
	users    storage.UserStore     // Budget deduction (nil = no budget tracking)
	budgetCk *notify.BudgetChecker // Budget threshold notifications (nil = no alerts)

	compatModels     []CompatModel // configured models for per-provider /models responses
	modelListJSON    atomic.Value  // cached []byte JSON response for /v1/models
	modelProviderMap atomic.Value  // cached map[string]string (model→provider) for O(1) lookups
	compatMu         sync.RWMutex  // guards compatModels reads during refresh

	// CRIT-3: Durable outbox for failed DeductSpend calls.
	spendOutbox *spendoutbox.Outbox

	// #277/#278: Per-request cost cap and per-model daily spend limits.
	config       Config        // retained for limit config access
	spendTracker *SpendTracker // per-user per-model daily spend tracker (nil if no limits)

	// #207: Model allowlist policy gate.
	policy *ModelPolicy

	// HIGH-2: Service account spend tracking (micro-USD for precision).
	saSpendMicroUSD atomic.Int64

	// Per-service-account in-process rate limiter.
	// SAs bypass user budget/rate-limit checks, so this is their only throttle.
	saRL *saRateLimiter

	// TOCTOU mitigation: tracks in-flight spend between CheckBudget and
	// DeductSpend. Without this, concurrent requests all pass CheckBudget
	// because none have been recorded in Firestore yet.
	pendingSpend *pendingSpendTracker

	// HIGH-5: Spans dropped at proxy semaphore (circuit breaker full).
	droppedSpans atomic.Int64

	// #333: Best-effort async ops dropped when asyncSem is full.
	droppedAsync atomic.Int64
}

// Config holds proxy configuration.
type Config struct {
	Providers      []Provider         `yaml:"providers"`
	ProjectID      string             `yaml:"project_id"`
	MaxRequestCost float64            `yaml:"max_request_cost_usd"` // Per-request cost cap (0 = disabled)
	DailyLimits    []SpendLimitConfig `yaml:"daily_limits"`         // Per-model daily spend limits
	Policy         *PolicyConfig      `yaml:"policy"`               // Model allowlist policy (nil = all allowed)

	// HTTP transport tuning for upstream LLM provider connections.
	MaxIdleConns        int `yaml:"max_idle_conns"`          // default: 200
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host"` // default: 50
	MaxConnsPerHost     int `yaml:"max_conns_per_host"`      // default: 100
}

// DefaultProviders returns the standard LLM provider configurations.
// Anthropic uses Vertex AI endpoint — set VERTEX_REGION and GCP_PROJECT env vars.
func DefaultProviders() []Provider {
	return []Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
		{Name: "google", UpstreamURL: "https://generativelanguage.googleapis.com"},
		// Gemini via OpenAI-compatible API. Use this with Cursor and other OpenAI-compat clients.
		{Name: "gemini-oai", UpstreamURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
		// Gemini via Vertex AI native endpoint. No format translation — client speaks
		// native Gemini API (generateContent/streamGenerateContent). Supports thought_signature
		// passthrough for Gemini 3.x thinking models. Candela handles GCP auth (ADC).
		// Use with @ai-sdk/google-vertex or any native Gemini SDK.
		{Name: "gemini-vertex", UpstreamURL: "https://us-central1-aiplatform.googleapis.com", Intercept: ptrBool(false)},
		// Anthropic via Vertex AI. Override upstream via config for your region/project:
		// https://{REGION}-aiplatform.googleapis.com/v1/projects/{PROJECT}/locations/{REGION}/publishers/anthropic/models
		{Name: "anthropic", UpstreamURL: "https://us-central1-aiplatform.googleapis.com"},
		// Anthropic via Vertex AI (native Messages API) — for Claude Code LLM gateway mode.
		// No format translation. Client speaks native Anthropic, routed via Vertex rawPredict.
		// Candela handles GCP auth (ADC) — no ANTHROPIC_API_KEY needed.
		// Use this with: ANTHROPIC_BASE_URL=http://localhost:8181/proxy/anthropic-vertex
		{Name: "anthropic-vertex", UpstreamURL: "https://us-central1-aiplatform.googleapis.com"},
		// Anthropic Direct — native Messages API passthrough to api.anthropic.com.
		// No format translation, no Vertex AI, no ADC. Client provides its own API key.
		// Use this for Claude Code, pencil.dev, and other tools that speak native Anthropic.
		{Name: "anthropic-direct", UpstreamURL: "https://api.anthropic.com"},
		// Mistral via Vertex AI (rawPredict). Uses MaaSPathRewriter with publisher "mistralai".
		{Name: "mistral", UpstreamURL: "https://us-central1-aiplatform.googleapis.com"},
		// DeepSeek R1 via Vertex AI OpenAI-compat endpoint (us-central1). Model prefix: deepseek-ai/
		{Name: "deepseek", UpstreamURL: "https://us-central1-aiplatform.googleapis.com"},
		// DeepSeek V3 via Vertex AI OpenAI-compat endpoint (global). Model prefix: deepseek-ai/
		{Name: "deepseek-v3", UpstreamURL: "https://aiplatform.googleapis.com"},
		// Qwen via Vertex AI OpenAI-compat endpoint (global). Model prefix: qwen/
		{Name: "qwen", UpstreamURL: "https://aiplatform.googleapis.com"},
		// Z.AI GLM via Vertex AI OpenAI-compat endpoint (global). Model prefix: zai-org/
		{Name: "zai", UpstreamURL: "https://aiplatform.googleapis.com"},
	}
}

// ptrBool returns a pointer to the given bool value.
// Used for setting optional *bool fields in struct literals.
func ptrBool(v bool) *bool { return &v }

// newUpstreamHTTPClient builds a production-tuned HTTP client for upstream LLM
// provider connections. The default http.DefaultTransport has MaxIdleConnsPerHost=2,
// which causes constant TCP/TLS connection churn under concurrent load to Vertex AI
// (*.aiplatform.googleapis.com). Each new connection requires a full TLS handshake
// (~100ms) instead of reusing an existing one.
//
// Follows the pattern established in pkg/runtime/helpers.go but tuned for
// cloud provider traffic rather than local runtimes.
func newUpstreamHTTPClient(cfg Config) *http.Client {
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 200
	}
	maxIdlePerHost := cfg.MaxIdleConnsPerHost
	if maxIdlePerHost <= 0 {
		maxIdlePerHost = 50
	}
	maxConnsPerHost := cfg.MaxConnsPerHost
	if maxConnsPerHost <= 0 {
		maxConnsPerHost = 100
	}

	slog.Info("proxy: upstream HTTP transport configured",
		"max_idle_conns", maxIdleConns,
		"max_idle_conns_per_host", maxIdlePerHost,
		"max_conns_per_host", maxConnsPerHost,
	)

	return &http.Client{
		Timeout: 5 * time.Minute, // LLM calls can be slow
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 2 * time.Minute, // reasoning models can take 60s+ before first byte
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdlePerHost,
			MaxConnsPerHost:       maxConnsPerHost,
			IdleConnTimeout:       120 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// New creates a new LLM proxy.
func New(cfg Config, submitter SpanSubmitter, calc *costcalc.Calculator) (*Proxy, error) {
	providers := make(map[string]Provider)
	breakers := make(map[string]*CircuitBreaker)
	cbCfg := DefaultCircuitBreakerConfig()
	for _, p := range cfg.Providers {
		providers[p.Name] = p
		breakers[p.Name] = NewCircuitBreaker(cbCfg)
	}

	p := &Proxy{
		providers:    providers,
		submitter:    submitter,
		calc:         calc,
		projectID:    cfg.ProjectID,
		config:       cfg,
		breakers:     breakers,
		spanSem:      make(chan struct{}, 200), // CRIT-16: cap concurrent span goroutines
		asyncSem:     make(chan struct{}, 50),  // #333: cap best-effort async goroutines
		saRL:         newSARateLimiter(0),      // default 120 RPM per SA
		pendingSpend: newPendingSpendTracker(),
		client:       newUpstreamHTTPClient(cfg),
	}

	// Initialize spend tracker if daily limits are configured.
	if len(cfg.DailyLimits) > 0 {
		p.spendTracker = NewSpendTracker()
	}

	// #207: Initialize model allowlist policy.
	policy, err := NewModelPolicy(cfg.Policy)
	if err != nil {
		return nil, fmt.Errorf("proxy: %w", err)
	}
	p.policy = policy

	return p, nil
}

// SetUserStore sets the optional UserStore for budget deduction.
func (p *Proxy) SetUserStore(users storage.UserStore) {
	p.users = users
}

// SetBudgetChecker sets the optional BudgetChecker for threshold notifications.
func (p *Proxy) SetBudgetChecker(ck *notify.BudgetChecker) {
	p.budgetCk = ck
}

// SetSpendOutbox sets the optional spend outbox for durable DeductSpend retries (CRIT-3).
func (p *Proxy) SetSpendOutbox(o *spendoutbox.Outbox) {
	p.spendOutbox = o
}

// DroppedSpans returns the total number of spans dropped at the proxy semaphore (HIGH-5).
func (p *Proxy) DroppedSpans() int64 {
	return p.droppedSpans.Load()
}

// SASpendMicroUSD returns total service account spend in micro-USD (HIGH-2).
func (p *Proxy) SASpendMicroUSD() int64 {
	return p.saSpendMicroUSD.Load()
}

// SetCachingMode updates the Anthropic caching strategy at runtime.
// Safe to call concurrently: p.providers is immutable after New() returns,
// so iteration requires no synchronization. The FormatTranslator.SetCachingMode
// method itself uses atomic operations.
func (p *Proxy) SetCachingMode(mode CachingMode) {
	for _, provider := range p.providers {
		if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok {
			ft.SetCachingMode(mode)
		}
	}
}

// GetCachingMode returns the current Anthropic caching mode. If multiple
// providers have translators, returns the first one found.
func (p *Proxy) GetCachingMode() CachingMode {
	for _, provider := range p.providers {
		if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok {
			return ft.GetCachingMode()
		}
	}
	return CachingOff
}

// SetCacheTTL updates the Anthropic cache TTL at runtime.
// This is safe to call concurrently from any goroutine.
func (p *Proxy) SetCacheTTL(ttl CacheTTL) {
	for _, provider := range p.providers {
		if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok {
			ft.SetCacheTTL(ttl)
		}
	}
}

// GetCacheTTL returns the current Anthropic cache TTL. If multiple
// providers have translators, returns the first one found.
func (p *Proxy) GetCacheTTL() CacheTTL {
	for _, provider := range p.providers {
		if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok {
			return ft.GetCacheTTL()
		}
	}
	return CacheTTL5m
}

// RegisterRoutes registers proxy routes on the given mux.
// Pattern: /proxy/{provider}/...
func (p *Proxy) RegisterRoutes(mux *http.ServeMux) {
	for name := range p.providers {
		prefix := "/proxy/" + name + "/"
		mux.HandleFunc(prefix, p.handleProxy)
		slog.Info("registered proxy route", "path", prefix)
	}
}

// ServeHTTP implements http.Handler, allowing the proxy to be used directly.
// The request URL path must be in the form /proxy/{provider}/...
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handleProxy(w, r)
}

// CompatModel maps a model ID to a provider name for LM Studio compat mode.
type CompatModel struct {
	ID       string `yaml:"id" json:"id"`
	Provider string `yaml:"provider" json:"provider"`
}

// RegisterCompatRoutes registers OpenAI-compatible routes at /v1/ (no /proxy/ prefix).
// This enables LM Studio, IntelliJ's LM Studio integration, and similar tools
// that expect an OpenAI-compatible API at the root path.
//
// GET /v1/models returns the configured model list.
// POST /v1/chat/completions routes to the correct provider based on the model name.
func (p *Proxy) RegisterCompatRoutes(mux *http.ServeMux, models []CompatModel) {
	// Initial model list build.
	p.setModels(models)

	// GET /v1/models — return the configured model list (OpenAI-compatible).
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if data, ok := p.modelListJSON.Load().([]byte); ok {
			_, _ = w.Write(data)
		} else {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		}
	})

	// GET /api/v0/models — LM Studio native API (used by IntelliJ's "Test Connection").
	mux.HandleFunc("GET /api/v0/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if data, ok := p.modelListJSON.Load().([]byte); ok {
			_, _ = w.Write(data)
		} else {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		}
	})

	// POST /v1/chat/completions — route to provider based on model name in body.
	// The model→provider lookup uses an atomic map for O(1) lock-free lookups.
	lookupProvider := func(modelID string) (string, bool) {
		if m, ok := p.modelProviderMap.Load().(map[string]string); ok {
			prov, found := m[modelID]
			return prov, found
		}
		return "", false
	}

	// POST /v1/chat/completions — route to provider based on model name in body.
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				ProxyErrorResponse(w, http.StatusRequestEntityTooLarge, "request body too large", "invalid_request_error")
			} else {
				ProxyErrorResponse(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
			}
			return
		}
		_ = r.Body.Close()
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			ProxyErrorResponse(w, http.StatusBadRequest, "missing or invalid model field", "invalid_request_error")
			return
		}

		providerName, ok := lookupProvider(req.Model)
		if !ok {
			// Try prefix-based alias resolution (e.g. "claude-sonnet-4" → "claude-sonnet-4-20250514").
			// Use the atomic map for lock-free iteration.
			if provMap, loaded := p.modelProviderMap.Load().(map[string]string); loaded {
				var matches []string
				for id := range provMap {
					if strings.HasPrefix(id, req.Model) {
						matches = append(matches, id)
					}
				}
				if len(matches) == 1 {
					resolved := matches[0]
					slog.Info("compat: resolved model alias", "from", req.Model, "to", resolved)
					providerName = provMap[resolved]
					ok = true
					// Rewrite the model field in the body so upstream gets the canonical ID.
					body = rewriteModelField(body, resolved)
				}
			}
		}
		if !ok {
			ProxyErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("unknown model: %s. Configure it in proxy.lmstudio.models", req.Model), "invalid_request_error")
			return
		}

		// Rewrite to internal proxy path and delegate to existing handler.
		r.URL.Path = "/proxy/" + providerName + "/v1/chat/completions"
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		p.handleProxy(w, r)
	})
}

// setModels filters models by pricing availability, stores them, and rebuilds
// the cached JSON response. Thread-safe for concurrent reads.
func (p *Proxy) setModels(models []CompatModel) {
	// Filter models: only surface models with known pricing.
	var pricedModels []CompatModel
	for _, m := range models {
		if p.calc != nil && !p.calc.HasPricing(m.Provider, m.ID) {
			slog.Warn("⚠️ hiding model from /v1/models — no pricing configured",
				"model", m.ID, "provider", m.Provider)
			continue
		}
		pricedModels = append(pricedModels, m)
	}

	if dropped := len(models) - len(pricedModels); dropped > 0 {
		slog.Info("model list filtered by pricing availability",
			"total", len(models), "visible", len(pricedModels), "hidden", dropped)
	}

	p.compatMu.Lock()
	p.compatModels = pricedModels
	p.compatMu.Unlock()

	// Build O(1) lookup map for the chat completions hot path.
	providerMap := make(map[string]string, len(pricedModels))
	for _, m := range pricedModels {
		providerMap[m.ID] = m.Provider
	}
	p.modelProviderMap.Store(providerMap)

	p.modelListJSON.Store(buildModelsResponse(pricedModels))
	slog.Info("📋 model list updated", "models", len(pricedModels))
}

// RefreshModels replaces the current model list with a new set of models.
// This is intended to be called periodically when the catalog changes
// (e.g. Firestore catalog refresh) without restarting the server.
func (p *Proxy) RefreshModels(models []CompatModel) {
	p.setModels(models)
}

func buildModelsResponse(models []CompatModel) []byte {
	type modelEntry struct {
		ID                string `json:"id"`
		Object            string `json:"object"`
		Created           int64  `json:"created"`
		OwnedBy           string `json:"owned_by"`
		Type              string `json:"type"`               // LM Studio: "llm", "vlm", "embeddings"
		Publisher         string `json:"publisher"`          // LM Studio: model publisher
		Arch              string `json:"arch"`               // LM Studio: model architecture (required by IntelliJ)
		CompatibilityType string `json:"compatibility_type"` // LM Studio: "gguf", "mlx", etc.
		State             string `json:"state"`              // LM Studio: "loaded", "not-loaded"
		MaxContextLength  int    `json:"max_context_length"` // LM Studio: context window size
	}
	type modelsResponse struct {
		Object string       `json:"object"`
		Data   []modelEntry `json:"data"`
	}

	resp := modelsResponse{Object: "list", Data: make([]modelEntry, 0, len(models))}
	for _, m := range models {
		resp.Data = append(resp.Data, modelEntry{
			ID:                m.ID,
			Object:            "model",
			Created:           1700000000,
			OwnedBy:           m.Provider,
			Type:              "llm",
			Publisher:         m.Provider,
			Arch:              "auto",
			CompatibilityType: "gguf",
			State:             "loaded",
			MaxContextLength:  128000,
		})
	}

	b, err := json.Marshal(resp)
	if err != nil {
		// Should never happen with simple string/int fields, but be safe.
		slog.Error("failed to marshal models response", "error", err)
		return []byte(`{"object":"list","data":[]}`)
	}
	return b
}

// rewriteModelFieldRe matches the JSON model key with optional whitespace around
// the colon (e.g. both `"model":"x"` and `"model" : "x"`) so the rewrite is
// robust to both compact and pretty-printed JSON payloads.
var rewriteModelFieldRe = regexp.MustCompile(`("model"\s*:\s*)"([^"\\]|\\.)*"`)

// rewriteModelField replaces the "model" field in a JSON request body with newModel.
// Targets the "model" key specifically to avoid corrupting user message
// content that might contain the model name as plain text.
// Handles optional whitespace around the JSON colon ("model": "value").
func rewriteModelField(body []byte, newModel string) []byte {
	// Find the current model value by parsing first.
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		return body
	}
	newJSON, _ := json.Marshal(newModel)
	// Replace the first occurrence only — prevents corrupting user message
	// content that might contain the same model name as plain text.
	replaced := false
	return rewriteModelFieldRe.ReplaceAllFunc(body, func(match []byte) []byte {
		if replaced {
			return match
		}
		replaced = true
		// Preserve the key+whitespace prefix (e.g. `"model" : `), replace only the value.
		keyPart := rewriteModelFieldRe.FindSubmatch(match)[1]
		return append(keyPart, newJSON...)
	})
}

// requestIDPattern validates that a request ID contains only safe characters
// (alphanumeric, hyphens) and is between 1-128 chars.
// This prevents log injection and trace poisoning via crafted X-Request-ID headers.
// Also used to validate X-Session-Id and X-Candela-Tenant-Id.
var requestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,128}$`)

// safeModelNameRe validates model names before they're interpolated into
// Vertex AI URL paths (e.g. /v1/projects/.../models/{model}:predict).
//
// SECURITY: Model names come from the client request body and are passed
// unsanitized into fmt.Sprintf URL paths in PathRewriter implementations
// (VertexAIMaaSPathRewriter, VertexAIGeminiOAIPathRewriter, etc.).
// Without this check, a malicious model name containing path traversal
// characters (e.g. "../../other-project/locations/us-central1/...") could
// redirect requests to arbitrary Vertex AI resources accessible to the
// server's service account — an SSRF vulnerability.
//
// Allowed: alphanumeric, hyphens, underscores, dots, colons, @, forward slash
// (covers model names like "gemini-2.5-flash", "claude-sonnet-4@20250514",
// "mistral-large-2411", "deepseek-ai/deepseek-chat")
//
// Forward slashes are allowed for org/model names (e.g. "Qwen/Qwen2.5-Coder"),
// but ".." sequences are explicitly blocked to prevent path traversal.
var safeModelNameRe = regexp.MustCompile(`^[a-zA-Z0-9._@:/\-]+$`)

// isSafeModelName validates a model name is safe for URL path interpolation.
// It checks both the character set (safeModelNameRe) and rejects ".."
// path traversal sequences that could escape the intended URL path.
func isSafeModelName(model string) bool {
	return safeModelNameRe.MatchString(model) && !strings.Contains(model, "..")
}

// tenantIDPattern delegates to attribution.IDPattern for backward compatibility
// with tests in this package.
var tenantIDPattern = attribution.IDPattern

// parseBaggage delegates to the shared attribution package.
// Kept as a package-level function for test compatibility.
func parseBaggage(header string) (tenantID, jobID string) {
	return attribution.ParseBaggage(header)
}

// parseBaggageHeaders delegates to the shared attribution package.
func parseBaggageHeaders(values []string) (tenantID, jobID string) {
	return attribution.ParseBaggageHeaders(values)
}

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Generate or accept request ID.
	// CRITICAL: Validate to prevent log/trace injection.
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" || !requestIDPattern.MatchString(requestID) {
		requestID = GenerateTraceID() // 32-char hex
	}

	// Accept session ID from candela-local (or other clients).
	// Validate to prevent log/trace injection — same rules as request ID.
	sessionID := r.Header.Get("X-Session-Id")
	if sessionID != "" && !requestIDPattern.MatchString(sessionID) {
		sessionID = "" // invalid → discard
	}

	// ── Tenant ID & Job ID extraction (for multitenant cost attribution) ──
	attr := attribution.FromRequest(r)
	tenantID, jobID := attr.TenantID, attr.JobID

	// Extract W3C Trace Context for trace correlation with OTel-instrumented
	// callers (e.g. Google ADK, LangChain). When present, the proxy span
	// becomes a child of the caller's span in a unified trace tree.
	traceCtx := parseTraceparent(r.Header.Get("Traceparent"))

	// ── Resolve effective end-user identity ──
	// Identity comes exclusively from the auth context (email or UID).
	// candela-local users authenticate with their own ADC credentials,
	// so the token carries their real email — no override header needed.
	caller := auth.FromContext(r.Context())
	var effectiveUserID string
	var isServiceAccount bool
	if caller != nil {
		effectiveUserID = caller.EffectiveID()
		isServiceAccount = strings.HasSuffix(effectiveUserID, ".gserviceaccount.com")
	}

	// ── Service account rate limiting ──
	// SAs bypass user-level rate limits and budget checks, so they need their
	// own throttle to prevent runaway automation from burning through quota.
	if isServiceAccount && p.saRL != nil {
		allowed, count, limit := p.saRL.Allow(effectiveUserID)
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"service account rate limit exceeded (%d/%d requests/min)","type":"rate_limit_exceeded","code":429}}`, count, limit)
			slog.Info("blocked SA request: rate limit exceeded",
				"sa", effectiveUserID, "count", count, "limit", limit)
			return
		}
	}

	// Extract provider from path: /proxy/{provider}/v1/...
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/proxy/"), "/", 2)
	if len(parts) < 2 {
		ProxyErrorResponse(w, http.StatusBadRequest, "invalid proxy path", "invalid_request_error")
		return
	}
	providerName := parts[0]
	upstreamPath := "/" + parts[1]

	provider, ok := p.providers[providerName]
	if !ok {
		ProxyErrorResponse(w, http.StatusBadRequest, "unknown provider", "invalid_request_error")
		return
	}

	// Check if user is admin — admins bypass rate limits and budget gates.
	var isAdmin bool
	if p.users != nil && effectiveUserID != "" && !isServiceAccount {
		if u, err := p.users.GetUser(r.Context(), effectiveUserID); err == nil && u != nil && u.Role == storage.RoleAdmin {
			isAdmin = true
		}
	}

	// ── Pre-flight budget check ──
	if p.users != nil && effectiveUserID != "" && !isServiceAccount && !isAdmin {
		// ── Rate limiting ──
		allowed, count, limit, rlErr := p.users.CheckRateLimit(r.Context(), effectiveUserID)
		if rlErr != nil {
			slog.Error("rate limit check failed, blocking request",
				"user_id", effectiveUserID, "error", rlErr)
			ProxyErrorResponse(w, http.StatusServiceUnavailable, "rate limit check unavailable — try again shortly", "service_error")
			return
		} else if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"rate limit exceeded (%d/%d requests/min)","type":"rate_limit_exceeded","code":429}}`, count, limit)
			slog.Info("blocked request: rate limit exceeded",
				"user_id", effectiveUserID,
				"count", count, "limit", limit)
			return
		}
		// Budget + pricing gates are applied after body read (below) so we
		// can extract the model name for a more accurate pre-flight estimate.
	}

	// Handle GET /v1/models — return configured models for this provider.
	// Build per-provider model list from the proxy's registered providers/models.
	if r.Method == http.MethodGet && strings.HasSuffix(upstreamPath, "/models") {
		// Collect models configured for this specific provider.
		p.compatMu.RLock()
		var providerModels []CompatModel
		for _, m := range p.compatModels {
			if m.Provider == providerName {
				providerModels = append(providerModels, m)
			}
		}
		p.compatMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildModelsResponse(providerModels))
		return
	}

	// Read the request body (capped at 10MB — returns 413 if exceeded).
	reqBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			ProxyErrorResponse(w, http.StatusRequestEntityTooLarge, "request body too large", "invalid_request_error")
		} else {
			ProxyErrorResponse(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		}
		return
	}
	_ = r.Body.Close()

	// Check if this is a streaming request (check BEFORE translation).
	isStreaming := isStreamingRequest(providerName, reqBody)

	// Native Google/Gemini providers determine streaming from the URL path
	// (:streamGenerateContent) rather than a body parameter. Detect this so
	// the PathRewriter generates the correct upstream action.
	if !isStreaming && (providerName == "google" || providerName == "gemini-vertex") {
		isStreaming = strings.Contains(upstreamPath, "streamGenerateContent")
	}

	// Extract model from request once — reused for pricing gate, body
	// enrichment, and path rewriting. For most providers the model is in
	// the request body; for "google" it's in the URL path
	// (e.g. /v1beta/models/gemini-2.5-flash:generateContent).
	requestModel, _ := extractRequestInfo(providerName, reqBody)
	if requestModel == "" {
		requestModel = extractModelFromURLPath(upstreamPath)
	}

	// ── Pricing gate (#6) — blocks unpriced cloud models universally ──
	// This runs for ALL requests (solo, team, local) to prevent untracked
	// API calls that would show $0 cost. Only "local" provider is exempt.
	if requestModel != "" && strings.ToLower(providerName) != "local" && !p.calc.HasPricing(pricingProvider(providerName), requestModel) {
		ProxyErrorResponse(w, http.StatusPaymentRequired, "no pricing configured for model "+requestModel+" — contact your admin", "pricing_not_configured")
		slog.Warn("blocked request: no pricing for model",
			"user_id", effectiveUserID, "provider", providerName, "model", requestModel)
		return
	}

	// ── Model allowlist policy gate (#207) ──
	// Blocks models not in the configured allowlist. Runs after pricing gate
	// and before budget checks. Empty/missing policy = all models allowed.
	if requestModel != "" && !p.policy.IsAllowed(providerName, requestModel) {
		ProxyErrorResponse(w, http.StatusForbidden, "model not allowed by policy", "forbidden")
		slog.Warn("model blocked by policy",
			"provider", providerName, "model", requestModel, "user", effectiveUserID)
		return
	}

	// ── Budget pre-flight with model-aware floor (#7) ──
	// Only applies in team mode (UserStore configured).
	var budgetReserved float64 // track reservation for Release in deductBudget
	if p.users != nil && effectiveUserID != "" && !isServiceAccount && !isAdmin {
		// #7: Budget check with a per-call floor so even a $0.001-remaining
		// user can't fire a $50 request. Floor = minimum meaningful API cost.
		const budgetCheckFloor = 0.001 // $0.001 — lower than any cloud model's minimum call
		check, err := p.users.CheckBudget(r.Context(), effectiveUserID, budgetCheckFloor)
		if err != nil {
			slog.Error("budget check failed, blocking request",
				"user_id", effectiveUserID, "error", err)
			ProxyErrorResponse(w, http.StatusServiceUnavailable, "budget check unavailable — try again shortly", "service_error")
			return
		} else if check == nil {
			slog.Error("budget check returned nil result, blocking request",
				"user_id", effectiveUserID)
			ProxyErrorResponse(w, http.StatusServiceUnavailable, "budget check failed — try again shortly", "service_error")
			return
		} else if !check.Allowed {
			ProxyErrorResponse(w, http.StatusPaymentRequired, "budget exhausted — contact your admin for a grant or budget increase", "insufficient_budget")
			slog.Info("blocked request: budget exhausted",
				"user_id", effectiveUserID, "remaining_usd", check.RemainingUSD)
			return
		}

		// TOCTOU mitigation: Firestore says remaining=$X, but other in-flight
		// requests may have already passed CheckBudget without DeductSpend yet.
		// Subtract their pending spend from the effective remaining balance.
		effectiveRemaining := check.RemainingUSD - p.pendingSpend.Get(effectiveUserID)
		if effectiveRemaining < budgetCheckFloor {
			ProxyErrorResponse(w, http.StatusPaymentRequired, "budget exhausted (concurrent requests in flight) — try again shortly", "insufficient_budget")
			slog.Info("blocked request: budget exhausted after pending spend",
				"user_id", effectiveUserID,
				"firestore_remaining", check.RemainingUSD,
				"pending_spend", p.pendingSpend.Get(effectiveUserID))
			return
		}

		// Reserve a conservative estimate so concurrent requests see lower balance.
		// We use the floor ($0.001) because we don't know actual cost yet.
		// This is a lower bound — the real cost will be higher, but even a
		// small reservation prevents unlimited concurrent overdraft.
		budgetReserved = budgetCheckFloor
		p.pendingSpend.Reserve(effectiveUserID, budgetReserved)
		// Release the reservation if we return before the response handlers
		// (handleStreamingResponse / handleStandardResponse) take ownership.
		defer func() {
			if budgetReserved > 0 {
				p.pendingSpend.Release(effectiveUserID, budgetReserved)
			}
		}()
	}

	// ── Per-request cost cap (#277) ──
	// Estimates the request cost from body size and rejects if it exceeds
	// the configured maximum. Runs for all users (solo + team mode).
	var estimatedCost float64
	if p.config.MaxRequestCost > 0 && requestModel != "" {
		estimatedCost = estimateRequestCost(p.calc, providerName, requestModel, reqBody)
		if estimatedCost > p.config.MaxRequestCost {
			ProxyErrorResponse(w, http.StatusPaymentRequired, fmt.Sprintf("estimated request cost $%.2f exceeds cap $%.2f for model %s",
				estimatedCost, p.config.MaxRequestCost, requestModel), "request_cost_exceeded")
			slog.Info("blocked request: estimated cost exceeds cap",
				"user_id", effectiveUserID, "provider", providerName,
				"model", requestModel, "estimated_usd", estimatedCost,
				"cap_usd", p.config.MaxRequestCost)
			return
		}
	}

	// ── Per-model daily spend limit (#278) ──
	// Checks in-memory per-user per-model daily spend against configured limits.
	// Uses "local" fallback for solo mode where effectiveUserID is empty.
	if p.spendTracker != nil && requestModel != "" {
		limitUser := effectiveUserID
		if limitUser == "" {
			limitUser = "local"
		}
		allowed, spent, limit := p.spendTracker.Check(limitUser, requestModel, p.config.DailyLimits, estimatedCost)
		if !allowed {
			ProxyErrorResponse(w, http.StatusPaymentRequired, fmt.Sprintf("daily spend limit for %s reached ($%.2f/$%.2f)",
				requestModel, spent, limit), "daily_limit_exceeded")
			slog.Info("blocked request: daily model spend limit exceeded",
				"user_id", limitUser, "model", requestModel,
				"spent_usd", spent, "limit_usd", limit)
			return
		}
	}

	// --- Translation layer ---
	// If the provider has a FormatTranslator, convert the request format
	// (e.g. OpenAI Chat Completions → Anthropic Messages).
	var translatedModel string
	upstreamBody := reqBody

	// Always strip Candela-internal headers before forwarding — they must
	// never leak to any upstream (Anthropic, Gemini, etc.).
	cachingOverride := r.Header.Get(CachingHeader)
	r.Header.Del(CachingHeader)
	ttlOverride := r.Header.Get(CacheTTLHeader)
	r.Header.Del(CacheTTLHeader)

	if provider.FormatTranslator != nil {
		// Per-request caching override via X-Candela-Caching / X-Candela-Cache-TTL headers.
		// Uses TranslateRequestWithModeAndTTL to avoid mutating shared translator
		// state, which would race with concurrent requests.
		if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok && (cachingOverride != "" || ttlOverride != "") {
			mode := ft.GetCachingMode()
			ttl := ft.GetCacheTTL()
			if cachingOverride != "" {
				mode = ParseCachingMode(cachingOverride)
			}
			if ttlOverride != "" {
				ttl = ParseCacheTTL(ttlOverride)
			}
			upstreamBody, translatedModel, err = ft.TranslateRequestWithModeAndTTL(reqBody, mode, ttl)
		} else {
			upstreamBody, translatedModel, err = provider.FormatTranslator.TranslateRequest(reqBody)
		}
		if err != nil {
			ProxyErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("request translation error: %v", err), "invalid_request_error")
			return
		}

		// For translated providers, also check streaming against the translated body.
		isStreaming = isStreamingRequest(providerName, upstreamBody)

		slog.Debug("translated request",
			"provider", providerName,
			"model", translatedModel,
			"streaming", isStreaming)
	}

	// --- Vertex AI body enrichment for native Anthropic passthrough ---
	// When there's no FormatTranslator (e.g. anthropic-vertex), the body is
	// forwarded as-is. But Vertex AI rawPredict requires:
	//   1. `anthropic_version` in the body (Claude Code sends it as a header)
	//   2. `model` NOT in the body (Vertex AI identifies model from URL path)
	// NOTE: Bedrock also has a PathRewriter but doesn't need anthropic_version
	// or model stripping — Bedrock accepts the model in the URL and ignores
	// extra body fields. Use RequestSigner to distinguish: Bedrock uses SigV4
	// signing, Vertex uses Bearer tokens.
	// NOTE: This block is scoped to Anthropic providers — Gemini providers with
	// PathRewriter have different body requirements (model prefix, no stripping).
	isAnthropicPassthrough := providerName == "anthropic-vertex"
	if provider.FormatTranslator == nil && provider.PathRewriter != nil && provider.RequestSigner == nil && isAnthropicPassthrough {
		var bodyMap map[string]interface{}
		if json.Unmarshal(upstreamBody, &bodyMap) == nil && bodyMap != nil {
			modified := false
			// Inject anthropic_version — Vertex AI rawPredict requires
			// a specific version, not the standard "2023-06-01" that
			// Claude Code / @ai-sdk/anthropic sends. Always override.
			wantVersion := provider.AnthropicVersion
			if wantVersion == "" {
				wantVersion = DefaultVertexAnthropicVersion
			}
			if v, _ := bodyMap["anthropic_version"].(string); v != wantVersion {
				bodyMap["anthropic_version"] = wantVersion
				modified = true
			}
			// Strip model — Vertex AI gets it from the URL path and
			// rejects extra fields in the body.
			if _, hasModel := bodyMap["model"]; hasModel {
				delete(bodyMap, "model")
				modified = true
			}
			if modified {
				if enriched, err := json.Marshal(bodyMap); err == nil {
					upstreamBody = enriched
				}
			}

			// Debug: log upstream body when CANDELA_DEBUG=proxy is set.
			if os.Getenv("CANDELA_DEBUG") == "proxy" {
				// Log top-level keys and cache_control presence for debugging.
				keys := make([]string, 0, len(bodyMap))
				for k := range bodyMap {
					keys = append(keys, k)
				}
				// Check system for cache_control.
				var systemSnippet string
				if sys, ok := bodyMap["system"]; ok {
					if b, err := json.Marshal(sys); err == nil {
						if len(b) > 500 {
							systemSnippet = string(b[:500]) + "..."
						} else {
							systemSnippet = string(b)
						}
					}
				}
				slog.Info("CANDELA_DEBUG: passthrough body",
					"provider", providerName,
					"keys", keys,
					"anthropic_version", bodyMap["anthropic_version"],
					"system_snippet", systemSnippet,
					"body_len", len(upstreamBody))
			}
		}
	}

	// --- Vertex AI Gemini model prefix injection ---
	// Vertex AI's OpenAI-compat endpoint requires model names to include the
	// publisher prefix (e.g. "google/gemini-2.5-flash"). Transparently inject
	// the "google/" prefix so clients can send bare model names.
	// Uses requestModel (extracted once above) to avoid redundant JSON parsing.
	// Uses rewriteModelField (regex) to avoid expensive JSON round-trip.
	// --- Vertex AI publisher prefix injection ---
	// Vertex AI's OpenAI-compat endpoint requires model names to include the
	// publisher prefix (e.g. "google/gemini-2.5-flash"). Transparently inject
	// the prefix so clients can send bare model names.
	if provider.PathRewriter != nil && requestModel != "" {
		switch providerName {
		case "gemini-oai":
			if !strings.HasPrefix(requestModel, "google/") {
				upstreamBody = rewriteModelField(upstreamBody, "google/"+requestModel)
			}
		case "deepseek", "deepseek-v3":
			if !strings.HasPrefix(requestModel, "deepseek-ai/") {
				upstreamBody = rewriteModelField(upstreamBody, "deepseek-ai/"+requestModel)
			}
		case "qwen":
			if !strings.HasPrefix(requestModel, "qwen/") {
				upstreamBody = rewriteModelField(upstreamBody, "qwen/"+requestModel)
			}
		case "zai":
			if !strings.HasPrefix(requestModel, "zai-org/") {
				upstreamBody = rewriteModelField(upstreamBody, "zai-org/"+requestModel)
			}
		}
	}

	// --- Path rewriting ---
	// If the provider has a PathRewriter, rewrite the upstream URL path
	// (e.g. Vertex AI project-scoped model endpoints).
	// Uses translatedModel (from FormatTranslator) or requestModel
	// (extracted once above from body or URL path).
	modelForPath := translatedModel
	if modelForPath == "" {
		modelForPath = requestModel
	}
	// SECURITY: Reject model names with path traversal characters before
	// they're interpolated into Vertex AI URL paths. Without this, a crafted
	// model name like "../../other-project/locations/.../models/gemini-2.5-flash"
	// could SSRF into arbitrary Vertex AI resources the SA has access to.
	// See safeModelNameRe definition for the full threat model.
	if modelForPath != "" && !isSafeModelName(modelForPath) {
		slog.Warn("blocked request: invalid model name (possible path traversal)",
			"model", modelForPath, "provider", providerName, "user_id", effectiveUserID)
		ProxyErrorResponse(w, http.StatusBadRequest, "invalid model name", "invalid_request_error")
		return
	}
	if provider.PathRewriter != nil && modelForPath != "" {
		upstreamPath = provider.PathRewriter.RewritePath(modelForPath, isStreaming)
	}

	// --- Stream usage injection ---
	// OpenAI/Gemini-OAI only include usage data in the final SSE chunk when
	// stream_options.include_usage is set. Without this, streaming responses
	// return 0 tokens. Inject it transparently so token counting always works.
	if isStreaming && provider.FormatTranslator == nil {
		upstreamBody = injectStreamUsageOption(providerName, upstreamBody)
	}

	// Build the upstream request.
	upstreamURL := provider.UpstreamURL + upstreamPath
	// SECURITY: Only forward client query params for passthrough providers
	// (openai, anthropic-direct) where the client provides their own API key.
	// For managed-auth providers (Vertex AI, Bedrock), strip the query string
	// to prevent parameter injection (?key=..., ?alt=media, etc.).
	if r.URL.RawQuery != "" && provider.TokenSource == nil && provider.RequestSigner == nil {
		upstreamURL += "?" + r.URL.RawQuery
	}

	// SECURITY: Use a detached context for the upstream request so that a
	// client disconnect doesn't cancel the upstream call. This ensures we
	// always receive the final usage chunk (with token counts) from the
	// upstream provider, preventing cost evasion by aborting mid-stream.
	// The 5-minute client timeout on p.client still applies.
	upstreamCtx, streamCancel := context.WithTimeout(context.WithoutCancel(r.Context()), maxStreamDuration)
	defer streamCancel()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		ProxyErrorResponse(w, http.StatusInternalServerError, "failed to create upstream request", "server_error")
		return
	}

	// Forward headers (auth, content-type, etc).
	forwardHeaders(r, upstreamReq, providerName, provider.TokenSource != nil || provider.RequestSigner != nil || provider.APIKey != "")

	// Pre-generate the span ID that buildSpan will use for this proxy span.
	// We need it now so the outgoing traceparent to the upstream LLM
	// references the sidecar's span as its parent.
	proxySpanID := GenerateSpanID()

	// Ensure we always have a trace context — either from the caller's
	// incoming traceparent or a freshly-generated root trace ID. This
	// guarantees every upstream request gets a traceparent, enabling
	// end-to-end trace correlation even when the caller has no OTel.
	if traceCtx == nil {
		traceCtx = &traceContext{traceID: GenerateTraceID()}
	}

	// Inject an outgoing traceparent so the upstream LLM API (if it
	// supports OTel) creates spans as children of the sidecar span.
	upstreamReq.Header.Set("Traceparent",
		fmt.Sprintf("00-%s-%s-01", traceCtx.traceID, proxySpanID))

	// --- Auth injection ---
	// RequestSigner (e.g., AWS SigV4) takes precedence over TokenSource (Bearer tokens).
	if provider.RequestSigner != nil {
		if signErr := provider.RequestSigner.SignRequest(r.Context(), upstreamReq, upstreamBody); signErr != nil {
			slog.Error("failed to sign request", "provider", providerName, "error", signErr)
			ProxyErrorResponse(w, http.StatusInternalServerError, "failed to sign upstream request", "server_error")
			return
		}
	} else if provider.TokenSource != nil {
		token, tokenErr := provider.TokenSource.Token()
		if tokenErr != nil {
			slog.Error("failed to get auth token", "provider", providerName, "error", tokenErr)
			ProxyErrorResponse(w, http.StatusInternalServerError, "failed to obtain cloud credentials", "server_error")
			return
		}
		upstreamReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	} else if provider.APIKey != "" {
		// Static API key auth (custom providers configured via YAML).
		if provider.AuthHeader != "" {
			upstreamReq.Header.Set(provider.AuthHeader, provider.APIKey)
		} else {
			upstreamReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
		}
	}

	// Propagate request ID to upstream.
	upstreamReq.Header.Set("X-Request-ID", requestID)

	// Execute the upstream request.
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		// CRITICAL: Don't leak internal URLs/DNS/network info to the client.
		slog.Error("upstream request failed", "provider", providerName, "error", err)
		ProxyErrorResponse(w, http.StatusBadGateway, "upstream provider unavailable", "upstream_error")
		p.recordCircuitBreaker(providerName, true)
		return
	}
	ttfb := time.Since(startTime)
	defer func() { _ = resp.Body.Close() }()

	// Record circuit breaker state based on upstream response.
	p.recordCircuitBreaker(providerName, resp.StatusCode >= 500)

	// Return request ID to the client.
	w.Header().Set("X-Request-ID", requestID)

	// Check circuit breaker — if open, skip observability but still forward.
	cbAllow := p.breakers[providerName].AllowRequest()

	// CRIT-17: Skip observability for utility endpoints (e.g. count_tokens,
	// tokenize) that don't generate tokens. Parsing their response as a chat
	// completion produces misleading 0-token spans and incorrect billing.
	// The proxy still forwards transparently — only span/billing is suppressed.
	if isUtilityEndpoint(upstreamPath) {
		cbAllow = false
		slog.Debug("skipping observability for utility endpoint",
			"provider", providerName, "path", upstreamPath, "request_id", requestID)
	}

	// ── Resolve effective cache TTL for Anthropic cost normalization (#191) ──
	// Determines whether cache creation tokens should be charged at 2.0× (1h)
	// or the default 1.25× (5m). Three sources, in priority order:
	//   1. Per-request header override (X-Candela-Cache-TTL)
	//   2. Translator config (AnthropicFormatTranslator.GetCacheTTL)
	//   3. Request body inspection (passthrough routes: cache_control.ttl)
	extendedTTL := false
	if ttlOverride != "" {
		extendedTTL = ParseCacheTTL(ttlOverride) == CacheTTL1h
	} else if ft, ok := provider.FormatTranslator.(*AnthropicFormatTranslator); ok {
		extendedTTL = ft.GetCacheTTL() == CacheTTL1h
	} else if provider.FormatTranslator == nil && isAnthropicProvider(providerName) {
		// Passthrough Anthropic routes — inspect request body.
		extendedTTL = extractAnthropicCacheTTL(reqBody)
	}

	// Transfer budget reservation ownership to the response handler.
	// Zero out so the deferred release guard (above) becomes a no-op.
	reserved := budgetReserved
	budgetReserved = 0

	if isStreaming && resp.StatusCode == http.StatusOK {
		p.handleStreamingResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, cbAllow, traceCtx, proxySpanID, extendedTTL, &reserved)
	} else {
		p.handleStandardResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, cbAllow, traceCtx, proxySpanID, extendedTTL, &reserved)
	}
}

// recordCircuitBreaker records success/failure for the provider's circuit breaker.
func (p *Proxy) recordCircuitBreaker(providerName string, failed bool) {
	cb, ok := p.breakers[providerName]
	if !ok {
		return
	}
	if failed {
		cb.RecordFailure()
		if cb.State() == CircuitOpen {
			slog.Warn("circuit breaker tripped",
				"provider", providerName,
				"state", cb.State().String())
		}
	} else {
		cb.RecordSuccess()
	}
}

func (p *Proxy) handleStandardResponse(
	w http.ResponseWriter, r *http.Request,
	resp *http.Response, provider Provider,
	reqBody []byte, startTime time.Time,
	ttfb time.Duration,
	requestID string,
	sessionID string,
	effectiveUserID string,
	tenantID string,
	jobID string,
	cbAllow bool,
	traceCtx *traceContext,
	proxySpanID string,
	extendedTTL bool, // true = Anthropic 1h TTL (2.0× cache creation rate)
	budgetReserved *float64, // pending-spend reservation to release after DeductSpend
) {
	// Read the full response (reject if over 10MB to prevent OOM under concurrent load).
	// CRIT-15: Previously 50MB — with 100 concurrent requests that's 5GB of heap.
	// Matches the 10MB request body limit for symmetry.
	const respLimit = int64(10 << 20)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, respLimit+1))
	if err != nil {
		ProxyErrorResponse(w, http.StatusBadGateway, "failed to read upstream response", "upstream_error")
		return
	}
	if int64(len(respBody)) > respLimit {
		ProxyErrorResponse(w, http.StatusBadGateway, "upstream response too large", "upstream_error")
		return
	}

	endTime := time.Now()

	// --- Response translation ---
	// Translate response back to client format if provider has a FormatTranslator.
	clientBody := respBody
	if provider.FormatTranslator != nil && resp.StatusCode == http.StatusOK {
		model, _ := extractRequestInfo(provider.Name, reqBody)
		translated, transErr := provider.FormatTranslator.TranslateResponse(respBody, model)
		if transErr != nil {
			slog.Error("response translation failed", "provider", provider.Name, "error", transErr)
			// Fall through with untranslated body rather than failing.
		} else {
			clientBody = translated
		}
	}

	// Forward response headers.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	// Fix content-length if we translated.
	if provider.FormatTranslator != nil && resp.StatusCode == http.StatusOK {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(clientBody)))
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(clientBody)

	// ── Budget deduction (SYNCHRONOUS) ──
	// Deduct BEFORE returning from the handler so the next request's
	// CheckBudget sees the updated spend. This adds ~30ms (one Firestore
	// transaction) AFTER the response is fully written — zero user-visible
	// latency. Previously this ran inside the async span goroutine, causing
	// CheckBudget to read stale spend data and allowing sequential calls to
	// overshoot the budget (e.g. $9.45 spent on a $5 limit).
	//
	// SECURITY: Budget deduction is decoupled from cbAllow. Even when the
	// circuit breaker is open, we must still deduct spend and release the
	// pending-spend reservation. Otherwise users get free LLM usage during
	// CB open windows, and leaked reservations progressively block users.
	if p.users != nil && effectiveUserID != "" {
		model, _ := extractRequestInfo(provider.Name, reqBody)
		_, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)
		// Normalize cached input tokens for accurate budget deduction.
		// All parsers return raw counts; the calculator applies provider-
		// and model-specific cache discounts (e.g. Anthropic 90% read,
		// Google 90% for 2.5+, OpenAI 50%).
		ct := extractCacheTokens(provider.Name, respBody)
		if model == "" {
			model = extractModelFromResponse(provider.Name, respBody)
		}
		inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)
		deductCtx, deductCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		p.deductBudget(deductCtx, provider, model, effectiveUserID, inputTokens, outputTokens)
		deductCancel()
		// Clear the pending-spend reservation now that DeductSpend has
		// recorded the actual cost in Firestore. The deferred Release in
		// ServeHTTP will see 0 and become a no-op.
		if budgetReserved != nil && *budgetReserved > 0 {
			p.pendingSpend.Release(effectiveUserID, *budgetReserved)
			*budgetReserved = 0
		}
	} else if p.spendTracker != nil {
		// Solo mode: no UserStore but spend tracker is active.
		// Record per-model spend so daily limits work without Firestore.
		model, _ := extractRequestInfo(provider.Name, reqBody)
		_, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)
		ct := extractCacheTokens(provider.Name, respBody)
		if model == "" {
			model = extractModelFromResponse(provider.Name, respBody)
		}
		inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)
		cost := p.calc.Calculate(pricingProvider(provider.Name), model, inputTokens, outputTokens)
		limitUser := effectiveUserID
		if limitUser == "" {
			limitUser = "local"
		}
		p.spendTracker.Record(limitUser, model, cost, p.config.DailyLimits)
	}

	// Create observability span (async — don't block the handler).
	// CRIT-16: Use bounded semaphore to prevent goroutine accumulation
	// under Firestore contention (previously unbounded).
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		select {
		case p.spanSem <- struct{}{}:
			safeGo(func() {
				defer func() { <-p.spanSem }()
				defer spanCancel()
				p.createSpan(spanCtx, provider, reqBody, respBody, startTime, endTime, resp.StatusCode, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, traceCtx, proxySpanID, extendedTTL)
			})
		default:
			spanCancel()
			p.droppedSpans.Add(1)
			slog.Warn("span dropped: too many pending", "provider", provider.Name, "request_id", requestID)
		}
	} else {
		slog.Debug("skipping span creation (circuit open)", "provider", provider.Name, "request_id", requestID)
	}
}

func (p *Proxy) handleStreamingResponse(
	w http.ResponseWriter, r *http.Request,
	resp *http.Response, provider Provider,
	reqBody []byte, startTime time.Time,
	ttfb time.Duration,
	requestID string,
	sessionID string,
	effectiveUserID string,
	tenantID string,
	jobID string,
	cbAllow bool,
	traceCtx *traceContext,
	proxySpanID string,
	extendedTTL bool, // true = Anthropic 1h TTL (2.0× cache creation rate)
	budgetReserved *float64, // pending-spend reservation to release after DeductSpend
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		ProxyErrorResponse(w, http.StatusInternalServerError, "streaming not supported", "server_error")
		return
	}

	// Forward response headers for SSE.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Determine model name for stream chunk translation.
	var streamModel string
	if provider.FormatTranslator != nil {
		streamModel, _ = extractRequestInfo(provider.Name, reqBody)
	}

	// Tee the stream: forward to client AND buffer for observability.
	// The buffer captures the full SSE payload so trace content in BigQuery
	// is never truncated and the usage chunk (at stream-end) is always
	// present for accurate token/cost attribution.
	//
	// Cap at 10MB (matching the standard response respLimit) to prevent
	// unbounded memory growth under concurrent load. 10MB comfortably fits
	// any realistic LLM streaming response; if exceeded, content is
	// truncated but the stream still forwards to the client uninterrupted.
	var streamBuffer bytes.Buffer
	const maxStreamCapture = 10 << 20 // 10MB — matches respLimit
	streamCapped := false
	buf := make([]byte, 4096)
	var ttft time.Duration
	isFirstChunk := true
	streamCompleted := false // tracks whether stream ended normally

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if isFirstChunk {
				ttft = time.Since(startTime)
				isFirstChunk = false
			}

			chunk := buf[:n]

			// Buffer raw upstream data for observability (before translation).
			if cbAllow && !streamCapped {
				if streamBuffer.Len()+n > maxStreamCapture {
					streamCapped = true
					slog.Warn("stream capture truncated at 10MB",
						"provider", provider.Name, "request_id", requestID)
				} else {
					streamBuffer.Write(chunk)
				}
			}

			// Translate chunk if provider has a FormatTranslator.
			if provider.FormatTranslator != nil {
				translated, transErr := provider.FormatTranslator.TranslateStreamChunk(chunk, streamModel)
				if transErr != nil {
					slog.Debug("stream chunk translation failed", "error", transErr)
					// Forward raw chunk on error.
					_, _ = w.Write(chunk)
				} else {
					_, _ = w.Write(translated)
				}
			} else {
				_, _ = w.Write(chunk)
			}
			flusher.Flush()
		}
		if err != nil {
			if err == io.EOF {
				streamCompleted = true
			}
			break
		}
	}

	endTime := time.Now()

	parseData := streamBuffer.Bytes()

	// Parse the accumulated stream to extract usage data.
	// Use bounded timeout to prevent goroutine leaks.
	streamStatus := storage.SpanStatusOK
	if !streamCompleted {
		streamStatus = storage.SpanStatusError
	}
	// ── Budget deduction (SYNCHRONOUS) — same rationale as handleStandardResponse.
	if p.users != nil && effectiveUserID != "" {
		model, _ := extractRequestInfo(provider.Name, reqBody)
		_, inputTokens, outputTokens := extractStreamingUsage(provider.Name, parseData)
		ct := extractStreamingCacheTokens(provider.Name, parseData)
		if model == "" {
			model = extractModelFromStreamingResponse(provider.Name, parseData)
		}
		inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)
		deductCtx, deductCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		p.deductBudget(deductCtx, provider, model, effectiveUserID, inputTokens, outputTokens)
		deductCancel()
		if budgetReserved != nil && *budgetReserved > 0 {
			p.pendingSpend.Release(effectiveUserID, *budgetReserved)
			*budgetReserved = 0
		}
	} else if p.spendTracker != nil {
		// Solo mode: record per-model spend for daily limits.
		model, _ := extractRequestInfo(provider.Name, reqBody)
		_, inputTokens, outputTokens := extractStreamingUsage(provider.Name, parseData)
		ct := extractStreamingCacheTokens(provider.Name, parseData)
		if model == "" {
			model = extractModelFromStreamingResponse(provider.Name, parseData)
		}
		inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)
		cost := p.calc.Calculate(pricingProvider(provider.Name), model, inputTokens, outputTokens)
		limitUser := effectiveUserID
		if limitUser == "" {
			limitUser = "local"
		}
		p.spendTracker.Record(limitUser, model, cost, p.config.DailyLimits)
	}

	// Create observability span (async — don't block the handler).
	// CRIT-16: Use bounded semaphore to prevent goroutine accumulation.
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		select {
		case p.spanSem <- struct{}{}:
			safeGo(func() {
				defer func() { <-p.spanSem }()
				defer spanCancel()
				p.createStreamingSpan(spanCtx, provider, reqBody, parseData, startTime, endTime, ttfb, ttft, requestID, sessionID, effectiveUserID, tenantID, jobID, streamStatus, traceCtx, proxySpanID, extendedTTL)
			})
		default:
			spanCancel()
			p.droppedSpans.Add(1)
			slog.Warn("streaming span dropped: too many pending", "provider", provider.Name, "request_id", requestID)
		}
	} else {
		slog.Debug("skipping streaming span (circuit open)", "provider", provider.Name, "request_id", requestID)
	}
}

// spanParams holds the parsed data needed to build an observability span.
// Both standard and streaming responses produce the same struct; the only
// difference is how they parse the upstream response.
type spanParams struct {
	provider         Provider
	model            string
	sessionID        string
	effectiveUserID  string // resolved end-user identity (auth email > auth UID)
	tenantID         string // downstream customer/tenant (from Baggage or header)
	jobID            string // experiment/job ID (from Baggage or header)
	inputContent     string
	outputContent    string
	reasoningContent string
	inputTokens      int64
	outputTokens     int64
	cacheTokens      CacheTokens // raw prompt cache breakdown
	startTime        time.Time
	endTime          time.Time
	status           storage.SpanStatus
	ttfb             time.Duration
	requestID        string
	extraAttrs       map[string]string // streaming-specific, status code, etc.
	namePrefix       string            // e.g. "openai.chat" or "openai.chat.stream"
	traceCtx         *traceContext     // W3C trace context from caller (nil = generate new)
	proxySpanID      string            // pre-generated span ID (for outgoing traceparent)
}

// deductBudget handles the synchronous budget deduction after a response is
// written. This MUST run in the request handler goroutine (not async) so that
// the next request's CheckBudget reads the updated spend from Firestore.
// Extracted from buildSpan to decouple billing timing from span creation.
func (p *Proxy) deductBudget(ctx context.Context, provider Provider, model, userID string, inputTokens, outputTokens int64) {
	if p.users == nil || userID == "" {
		return
	}
	if strings.HasSuffix(userID, ".gserviceaccount.com") {
		// HIGH-2: Log SA spend instead of silently skipping.
		totalTokens := inputTokens + outputTokens
		cost := p.calc.Calculate(pricingProvider(provider.Name), model, inputTokens, outputTokens)
		if cost > 0 || totalTokens > 0 {
			p.saSpendMicroUSD.Add(int64(math.Round(cost * 1_000_000)))
			slog.Info("sa_spend: service account usage",
				"sa_id", userID,
				"provider", provider.Name,
				"model", model,
				"cost_usd", cost,
				"input_tokens", inputTokens,
				"output_tokens", outputTokens,
				"total_tokens", totalTokens,
			)
		}
		return
	}

	totalTokens := inputTokens + outputTokens
	cost := p.calc.Calculate(pricingProvider(provider.Name), model, inputTokens, outputTokens)

	// #278: Record per-model daily spend in the in-memory tracker.
	// This must happen even if DeductSpend fails, since the upstream
	// response was already forwarded (spend happened, we're tracking it).
	if p.spendTracker != nil && cost > 0 {
		p.spendTracker.Record(userID, model, cost, p.config.DailyLimits)
	}

	// #10: DeductSpend is called when CostUSD>0 OR TotalTokens>0.
	// The TotalTokens>0 case handles: unknown cloud models (cost=$0 from pricing
	// gap), streaming calls where the buffer cap lost the usage chunk, and any
	// other zero-cost path. We still want AllTokensUsed incremented for them.
	if cost <= 0 && totalTokens <= 0 {
		return
	}

	// CRIT-13: retry DeductSpend up to 3 times with exponential backoff.
	const maxDeductAttempts = 3
	var deductErr error
	for attempt := 0; attempt < maxDeductAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * 200 * time.Millisecond
			slog.Warn("deduct_spend: retrying after failure",
				"user_id", userID,
				"attempt", attempt+1,
				"backoff_ms", backoff.Milliseconds(),
				"error", deductErr)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
		deductErr = p.users.DeductSpend(ctx, userID, cost, totalTokens)
		if deductErr == nil {
			break
		}
	}
	if deductErr != nil {
		// CRIT-3: Queue to durable outbox instead of silently dropping.
		if p.spendOutbox != nil {
			if qErr := p.spendOutbox.Enqueue(ctx, spendoutbox.SpendRecord{
				UserID:  userID,
				CostUSD: cost,
				Tokens:  totalTokens,
			}); qErr != nil {
				slog.Error("deduct_spend: CRITICAL — failed to enqueue to outbox",
					"user_id", userID, "cost_usd", cost, "tokens", totalTokens,
					"enqueue_error", qErr, "original_error", deductErr)
			} else {
				slog.Warn("deduct_spend: queued to outbox for retry",
					"user_id", userID, "cost_usd", cost, "tokens", totalTokens,
					"attempts", maxDeductAttempts, "error", deductErr)
			}
		} else {
			slog.Error("deduct_spend: all retries exhausted — spend not recorded (no outbox)",
				"user_id", userID, "cost_usd", cost, "tokens", totalTokens,
				"attempts", maxDeductAttempts, "error", deductErr)
		}
		return
	}

	// Best-effort: update the user's last-active timestamp (proxy usage).
	select {
	case p.asyncSem <- struct{}{}:
		safeGo(func() {
			defer func() { <-p.asyncSem }()
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := p.users.TouchLastActive(bgCtx, userID); err != nil {
				slog.Warn("touch_last_active failed", "user_id", userID, "error", err)
			}
		})
	default:
		p.droppedAsync.Add(1)
		slog.Debug("async op dropped: too many pending", "op", "touch_last_active", "user_id", userID)
	}

	// Async: threshold notification is best-effort.
	if p.budgetCk != nil {
		select {
		case p.asyncSem <- struct{}{}:
			safeGo(func() {
				defer func() { <-p.asyncSem }()
				bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				budget, err := p.users.GetBudget(bgCtx, userID)
				if err == nil && budget != nil && budget.LimitUSD > 0 {
					var email string
					user, uerr := p.users.GetUser(bgCtx, userID)
					if uerr == nil && user != nil {
						email = user.Email
					}
					p.budgetCk.CheckAndNotify(bgCtx, userID, email, budget.PeriodKey,
						notify.DeductResult{
							SpentUSD: budget.SpentUSD,
							LimitUSD: budget.LimitUSD,
						})
				}
			})
		default:
			p.droppedAsync.Add(1)
			slog.Debug("async op dropped: too many pending", "op", "budget_notify", "user_id", userID)
		}
	}
}

// buildSpan constructs a storage.Span from parsed parameters and submits it.
// Budget deduction is handled separately by deductBudget (called synchronously
// in the request handler) — buildSpan only handles span creation and storage.
func (p *Proxy) buildSpan(ctx context.Context, params spanParams) {
	totalTokens := params.inputTokens + params.outputTokens
	pprov := pricingProvider(params.provider.Name)
	cost := p.calc.Calculate(pprov, params.model, params.inputTokens, params.outputTokens)

	// Snapshot point-in-time pricing so historical spans retain the rates
	// that were active at request time (enables cost auditing, #321).
	// Use ResolveEffective to capture the tier-adjusted rates that were
	// actually used for cost calculation (e.g. Gemini 2.5 Pro >200K tokens).
	var inputRate, outputRate, discountPct, globalDisc float64
	if pricing, ok := p.calc.ResolveEffective(pprov, params.model, params.inputTokens); ok {
		inputRate = pricing.InputPerMillion
		outputRate = pricing.OutputPerMillion
		discountPct = pricing.DiscountPercent
	}
	globalDisc = p.calc.GlobalDiscount()

	attrs := map[string]string{
		"proxy.upstream":  params.provider.UpstreamURL,
		"http.ttfb_ms":    fmt.Sprintf("%d", params.ttfb.Milliseconds()),
		"http.request_id": params.requestID,
	}
	for k, v := range params.extraAttrs {
		attrs[k] = v
	}
	// Expose raw cache token counts as attributes so the UI can show
	// cache utilization regardless of provider (Anthropic, OpenAI, Google).
	if params.cacheTokens.CacheReadTokens > 0 {
		attrs["gen_ai.usage.cache_read_tokens"] = fmt.Sprintf("%d", params.cacheTokens.CacheReadTokens)
	}
	if params.cacheTokens.CacheCreationTokens > 0 {
		attrs["gen_ai.usage.cache_creation_tokens"] = fmt.Sprintf("%d", params.cacheTokens.CacheCreationTokens)
	}

	// Use caller's trace context if present (W3C Trace Context propagation).
	// This nests the proxy span under the caller's OTel span.
	traceID := GenerateTraceID()
	parentSpanID := ""
	if params.traceCtx != nil {
		traceID = params.traceCtx.traceID
		parentSpanID = params.traceCtx.parentSpanID
	}

	span := storage.Span{
		SpanID:       params.proxySpanID,
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		Name:         params.namePrefix,
		Kind:         storage.SpanKindLLM,
		Status:       params.status,
		StartTime:    params.startTime,
		EndTime:      params.endTime,
		Duration:     params.endTime.Sub(params.startTime),
		ProjectID:    p.projectID,
		GenAI: &storage.GenAIAttributes{
			Model:               params.model,
			Provider:            params.provider.Name,
			InputTokens:         params.inputTokens,
			OutputTokens:        params.outputTokens,
			TotalTokens:         totalTokens,
			CostUSD:             cost,
			InputContent:        params.inputContent,
			OutputContent:       params.outputContent,
			ReasoningContent:    params.reasoningContent,
			CacheReadTokens:     params.cacheTokens.CacheReadTokens,
			CacheCreationTokens: params.cacheTokens.CacheCreationTokens,
			InputRate:           inputRate,
			OutputRate:          outputRate,
			DiscountPercent:     discountPct,
			GlobalDiscount:      globalDisc,
		},
		Attributes: attrs,
	}

	// Set user ID for per-user attribution (resolved once in handleProxy).
	span.UserID = params.effectiveUserID

	// Set session ID for conversation grouping.
	span.SessionID = params.sessionID

	// Set tenant ID for multitenant cost attribution.
	span.TenantID = params.tenantID

	// Set job ID for experiment/job-level cost attribution.
	span.JobID = params.jobID

	if p.submitter != nil {
		p.submitter.SubmitBatch([]storage.Span{span})
	}

	slog.Debug("proxied LLM call",
		"provider", params.provider.Name,
		"model", params.model,
		"tokens", totalTokens,
		"cost_usd", cost,
		"latency", params.endTime.Sub(params.startTime),
		"request_id", params.requestID,
		"streaming", params.extraAttrs["proxy.streaming"] == "true",
	)
}

func (p *Proxy) createSpan(
	ctx context.Context, provider Provider,
	reqBody, respBody []byte,
	startTime, endTime time.Time,
	statusCode int,
	ttfb time.Duration,
	requestID string,
	sessionID string,
	effectiveUserID string,
	tenantID string,
	jobID string,
	traceCtx *traceContext,
	proxySpanID string,
	extendedTTL bool,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)
	ct := extractCacheTokens(provider.Name, respBody)

	// For Google native, model is in the URL path, not the body.
	// Fall back to the response's modelVersion field.
	if model == "" {
		model = extractModelFromResponse(provider.Name, respBody)
	}

	// Extract <think> tags from output content (#312).
	// Only for known reasoning models — prevents corrupting legitimate
	// <think> content in non-reasoning model output (e.g. code examples).
	var reasoningContent string
	if isReasoningModel(model) {
		outputContent, reasoningContent = extractThinking(outputContent)
	}

	// Normalize cached input tokens via the calculator (handles all providers).
	// extendedTTL applies 2.0× for Anthropic 1-hour cache creation (#191).
	inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)

	status := storage.SpanStatusOK
	if statusCode >= 400 {
		status = storage.SpanStatusError
	}

	p.buildSpan(ctx, spanParams{
		provider:         provider,
		model:            model,
		effectiveUserID:  effectiveUserID,
		tenantID:         tenantID,
		jobID:            jobID,
		inputContent:     inputContent,
		outputContent:    outputContent,
		reasoningContent: reasoningContent,
		inputTokens:      inputTokens,
		outputTokens:     outputTokens,
		cacheTokens:      ct,
		startTime:        startTime,
		endTime:          endTime,
		status:           status,
		ttfb:             ttfb,
		requestID:        requestID,
		sessionID:        sessionID,
		namePrefix:       fmt.Sprintf("%s.chat", provider.Name),
		traceCtx:         traceCtx,
		proxySpanID:      proxySpanID,
		extraAttrs: map[string]string{
			"http.status": fmt.Sprintf("%d", statusCode),
		},
	})
}

func (p *Proxy) createStreamingSpan(
	ctx context.Context, provider Provider,
	reqBody, streamData []byte,
	startTime, endTime time.Time,
	ttfb time.Duration,
	ttft time.Duration,
	requestID string,
	sessionID string,
	effectiveUserID string,
	tenantID string,
	jobID string,
	streamStatus storage.SpanStatus,
	traceCtx *traceContext,
	proxySpanID string,
	extendedTTL bool,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractStreamingUsage(provider.Name, streamData)
	ct := extractStreamingCacheTokens(provider.Name, streamData)

	// For Google native, model is in the URL path, not the body.
	// Fall back to the response's modelVersion field.
	if model == "" {
		model = extractModelFromStreamingResponse(provider.Name, streamData)
	}

	// Extract <think> tags from streaming output content (#312).
	// Only for known reasoning models — prevents corrupting legitimate
	// <think> content in non-reasoning model output (e.g. code examples).
	var reasoningContent string
	if isReasoningModel(model) {
		outputContent, reasoningContent = extractThinking(outputContent)
	}

	// Normalize cached input tokens via the calculator (handles all providers).
	// extendedTTL applies 2.0× for Anthropic 1-hour cache creation (#191).
	inputTokens = p.calc.NormalizeCachedInputWithTTL(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens, extendedTTL)

	p.buildSpan(ctx, spanParams{
		provider:         provider,
		model:            model,
		effectiveUserID:  effectiveUserID,
		tenantID:         tenantID,
		jobID:            jobID,
		inputContent:     inputContent,
		outputContent:    outputContent,
		reasoningContent: reasoningContent,
		inputTokens:      inputTokens,
		outputTokens:     outputTokens,
		cacheTokens:      ct,
		startTime:        startTime,
		endTime:          endTime,
		status:           streamStatus,
		ttfb:             ttfb,
		requestID:        requestID,
		sessionID:        sessionID,
		namePrefix:       fmt.Sprintf("%s.chat.stream", provider.Name),
		traceCtx:         traceCtx,
		proxySpanID:      proxySpanID,
		extraAttrs: map[string]string{
			"proxy.streaming": "true",
			"llm.ttft_ms":     fmt.Sprintf("%d", ttft.Milliseconds()),
		},
	})
}

// --- Header forwarding ---

func forwardHeaders(src *http.Request, dst *http.Request, provider string, managedAuth bool) {
	// Only forward client Authorization for passthrough providers.
	// For providers where the proxy manages auth (TokenSource/RequestSigner),
	// skip forwarding to prevent client auth leaking if token fetch fails.
	if !managedAuth {
		if auth := src.Header.Get("Authorization"); auth != "" {
			dst.Header.Set("Authorization", auth)
		}
	}

	// Always forward these.
	for _, h := range []string{"Content-Type", "Accept"} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}

	// W3C Trace Context — enables end-to-end distributed tracing.
	// We forward Tracestate as-is, but Traceparent is NOT forwarded
	// directly — instead, buildSpan constructs an updated traceparent
	// with the sidecar's span ID as the new parent. This is handled
	// after span ID generation. See setOutgoingTraceparent.
	if v := src.Header.Get("Tracestate"); v != "" {
		dst.Header.Set("Tracestate", v)
	}

	// Provider-specific headers.
	switch provider {
	case "openai":
		// OpenAI uses Authorization: Bearer — already forwarded above.
	case "anthropic":
		// Anthropic via Vertex AI uses Authorization: Bearer (ADC token).
		// Direct API uses X-Api-Key. Forward both for flexibility.
		for _, h := range []string{"X-Api-Key", "Anthropic-Version", "Anthropic-Beta"} {
			if v := src.Header.Get(h); v != "" {
				dst.Header.Set(h, v)
			}
		}
	case "anthropic-direct":
		// Native Anthropic Messages API — forward all required headers.
		// Claude Code requires anthropic-beta and anthropic-version to be forwarded.
		for _, h := range []string{"X-Api-Key", "Anthropic-Version", "Anthropic-Beta"} {
			if v := src.Header.Get(h); v != "" {
				dst.Header.Set(h, v)
			}
		}
	case "anthropic-vertex":
		// Native Anthropic Messages API routed via Vertex AI.
		// Auth is handled by ADC TokenSource — no client API key needed.
		// DO NOT forward Anthropic-Version or Anthropic-Beta headers:
		// - anthropic_version is set in the body (vertex-2023-10-16)
		// - Anthropic-Beta headers cause Vertex AI to reject or silently
		//   ignore features like prompt caching.
	case "google":
		// Google uses API key in query params or Authorization header — both forwarded.
	}
}

// --- Request parsing (delegated to per-provider parsers) ---

func isStreamingRequest(provider string, body []byte) bool {
	return getParser(provider).IsStreaming(body)
}

func extractRequestInfo(provider string, body []byte) (model, content string) {
	return getParser(provider).ParseRequest(body)
}

func extractResponseInfo(provider string, body []byte) (content string, inputTokens, outputTokens int64) {
	return getParser(provider).ParseResponse(body)
}

func extractStreamingUsage(provider string, data []byte) (content string, inputTokens, outputTokens int64) {
	return getParser(provider).ParseStreamingResponse(data)
}

// --- Helpers ---

func toInt64(v interface{}) int64 {
	var result int64
	switch n := v.(type) {
	case float64:
		result = int64(n)
	case int64:
		result = n
	case json.Number:
		result, _ = n.Int64()
	default:
		return 0
	}
	// Token counts are always non-negative; clamp to 0 to guard against
	// malformed upstream responses (discovered by fuzzing).
	if result < 0 {
		return 0
	}
	return result
}

// isUtilityEndpoint returns true for API paths that are non-generative
// utility calls (e.g. count_tokens, tokenize). These endpoints don't
// produce chat completions, so parsing their response as one yields
// misleading 0-token spans. The proxy still forwards them transparently.
func isUtilityEndpoint(path string) bool {
	return strings.HasSuffix(path, "/count_tokens") ||
		strings.HasSuffix(path, "/tokenize") ||
		strings.HasSuffix(path, "/models") ||
		path == "/v1/models"
}

// extractModelFromURLPath extracts a model name from a Google API URL path.
// Supports patterns like:
//
//	/v1beta/models/gemini-2.5-flash:generateContent → gemini-2.5-flash
//	/v1/models/gemini-2.5-pro:streamGenerateContent → gemini-2.5-pro
func extractModelFromURLPath(path string) string {
	const marker = "/models/"
	idx := strings.LastIndex(path, marker)
	if idx < 0 {
		return ""
	}
	rest := path[idx+len(marker):]
	// Strip the method suffix (e.g. ":generateContent").
	if colon := strings.Index(rest, ":"); colon >= 0 {
		rest = rest[:colon]
	}
	return rest
}

// pricingProvider maps proxy provider names to their cost calculator
// provider key. Most providers use their own name; aliases like
// "gemini-vertex" map to the canonical provider ("google") whose
// pricing table is authoritative.
func pricingProvider(provider string) string {
	switch provider {
	case "gemini-vertex", "gemini-oai":
		return "google"
	case "anthropic-vertex", "anthropic-direct", "anthropic-bedrock":
		return "anthropic"
	default:
		return provider
	}
}
