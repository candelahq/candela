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
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"log/slog"

	"golang.org/x/oauth2"

	"github.com/candelahq/candela/pkg/attribution"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/notify"
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

// Provider defines an LLM API provider configuration.
type Provider struct {
	Name        string `yaml:"name"`     // "openai", "google", "anthropic", "gemini-oai"
	UpstreamURL string `yaml:"upstream"` // e.g. "https://api.openai.com"

	// FormatTranslator handles format translation (nil = transparent passthrough).
	FormatTranslator FormatTranslator `yaml:"-"`

	// PathRewriter rewrites upstream URL paths (nil = forward client path).
	PathRewriter PathRewriter `yaml:"-"`

	// TokenSource provides auto-refreshing auth tokens (e.g. GCP ADC). Nil = forward client auth.
	TokenSource oauth2.TokenSource `yaml:"-"`

	// AnthropicVersion overrides the anthropic_version injected into Vertex AI
	// rawPredict bodies. Empty = DefaultVertexAnthropicVersion.
	AnthropicVersion string `yaml:"-"`
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

	// Optional dependencies for team-mode features.
	users    storage.UserStore     // Budget deduction (nil = no budget tracking)
	budgetCk *notify.BudgetChecker // Budget threshold notifications (nil = no alerts)

	compatModels []CompatModel // configured models for per-provider /models responses
}

// Config holds proxy configuration.
type Config struct {
	Providers []Provider `yaml:"providers"`
	ProjectID string     `yaml:"project_id"`
}

// DefaultProviders returns the standard LLM provider configurations.
// Anthropic uses Vertex AI endpoint — set VERTEX_REGION and GCP_PROJECT env vars.
func DefaultProviders() []Provider {
	return []Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
		{Name: "google", UpstreamURL: "https://generativelanguage.googleapis.com"},
		// Gemini via OpenAI-compatible API. Use this with Cursor and other OpenAI-compat clients.
		{Name: "gemini-oai", UpstreamURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
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
	}
}

// New creates a new LLM proxy.
func New(cfg Config, submitter SpanSubmitter, calc *costcalc.Calculator) *Proxy {
	providers := make(map[string]Provider)
	breakers := make(map[string]*CircuitBreaker)
	cbCfg := DefaultCircuitBreakerConfig()
	for _, p := range cfg.Providers {
		providers[p.Name] = p
		breakers[p.Name] = NewCircuitBreaker(cbCfg)
	}

	return &Proxy{
		providers: providers,
		submitter: submitter,
		calc:      calc,
		projectID: cfg.ProjectID,
		breakers:  breakers,
		spanSem:   make(chan struct{}, 200), // CRIT-16: cap concurrent span goroutines
		client: &http.Client{
			Timeout: 5 * time.Minute, // LLM calls can be slow
		},
	}
}

// SetUserStore sets the optional UserStore for budget deduction.
func (p *Proxy) SetUserStore(users storage.UserStore) {
	p.users = users
}

// SetBudgetChecker sets the optional BudgetChecker for threshold notifications.
func (p *Proxy) SetBudgetChecker(ck *notify.BudgetChecker) {
	p.budgetCk = ck
}

// SetCachingMode updates the Anthropic caching strategy at runtime.
// This is safe to call concurrently from any goroutine.
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
	// Filter models: only surface models with known pricing.
	// This ensures clients never discover a model that would show $0.00 cost.
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

	// Build the /v1/models response once at startup.
	modelList := buildModelsResponse(pricedModels)

	// Store models for per-provider /models responses in handleProxy.
	p.compatModels = pricedModels

	// GET /v1/models — return the configured model list (OpenAI-compatible).
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelList)
	})

	// GET /api/v0/models — LM Studio native API (used by IntelliJ's "Test Connection").
	mux.HandleFunc("GET /api/v0/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelList)
	})

	// Build model→provider lookup.
	modelToProvider := make(map[string]string, len(pricedModels))
	for _, m := range pricedModels {
		modelToProvider[m.ID] = m.Provider
	}

	// POST /v1/chat/completions — route to provider based on model name in body.
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
			}
			return
		}
		_ = r.Body.Close()

		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"missing or invalid model field","type":"invalid_request_error"}}`))
			return
		}

		providerName, ok := modelToProvider[req.Model]
		if !ok {
			// Try prefix-based alias resolution (e.g. "claude-sonnet-4" → "claude-sonnet-4-20250514").
			var matches []string
			for id := range modelToProvider {
				if strings.HasPrefix(id, req.Model) {
					matches = append(matches, id)
				}
			}
			if len(matches) == 1 {
				resolved := matches[0]
				slog.Info("compat: resolved model alias", "from", req.Model, "to", resolved)
				providerName = modelToProvider[resolved]
				ok = true
				// Rewrite the model field in the body so upstream gets the canonical ID.
				body = rewriteModelField(body, resolved)
			}
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			errResp, _ := json.Marshal(map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("unknown model: %s. Configure it in proxy.lmstudio.models", req.Model),
					"type":    "invalid_request_error",
				},
			})
			_, _ = w.Write(errResp)
			return
		}

		// Rewrite to internal proxy path and delegate to existing handler.
		r.URL.Path = "/proxy/" + providerName + "/v1/chat/completions"
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		p.handleProxy(w, r)
	})
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

	resp := modelsResponse{Object: "list"}
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
		requestID = generateTraceID() // 32-char hex
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

	// Extract provider from path: /proxy/{provider}/v1/...
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/proxy/"), "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid proxy path", http.StatusBadRequest)
		return
	}
	providerName := parts[0]
	upstreamPath := "/" + parts[1]

	provider, ok := p.providers[providerName]
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	// ── Pre-flight budget check ──
	if p.users != nil && effectiveUserID != "" && !isServiceAccount {
		// ── Rate limiting ──
		allowed, count, limit, rlErr := p.users.CheckRateLimit(r.Context(), effectiveUserID)
		if rlErr != nil {
			slog.Warn("rate limit check failed, allowing request",
				"user_id", effectiveUserID, "error", rlErr)
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
		var providerModels []CompatModel
		for _, m := range p.compatModels {
			if m.Provider == providerName {
				providerModels = append(providerModels, m)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildModelsResponse(providerModels))
		return
	}

	// Read the request body (capped at 10MB — returns 413 if exceeded).
	reqBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
		}
		return
	}
	_ = r.Body.Close()

	// Check if this is a streaming request (check BEFORE translation).
	isStreaming := isStreamingRequest(providerName, reqBody)

	// ── Pricing gate (#6) — blocks unpriced cloud models universally ──
	// This runs for ALL requests (solo, team, local) to prevent untracked
	// API calls that would show $0 cost. Only "local" provider is exempt.
	{
		model, _ := extractRequestInfo(providerName, reqBody)
		if model != "" && strings.ToLower(providerName) != "local" && !p.calc.HasPricing(providerName, model) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			errBody, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": "no pricing configured for model " + model + " — contact your admin",
					"type":    "pricing_not_configured",
					"code":    402,
				},
			})
			_, _ = w.Write(errBody)
			slog.Warn("blocked request: no pricing for model",
				"user_id", effectiveUserID, "provider", providerName, "model", model)
			return
		}
	}

	// ── Budget pre-flight with model-aware floor (#7) ──
	// Only applies in team mode (UserStore configured).
	if p.users != nil && effectiveUserID != "" && !isServiceAccount {
		// #7: Budget check with a per-call floor so even a $0.001-remaining
		// user can't fire a $50 request. Floor = minimum meaningful API cost.
		const budgetCheckFloor = 0.001 // $0.001 — lower than any cloud model's minimum call
		check, err := p.users.CheckBudget(r.Context(), effectiveUserID, budgetCheckFloor)
		if err != nil {
			slog.Warn("budget check failed, allowing request",
				"user_id", effectiveUserID, "error", err)
		} else if !check.Allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"message":"budget exhausted — contact your admin for a grant or budget increase","type":"insufficient_budget","code":402}}`))
			slog.Info("blocked request: budget exhausted",
				"user_id", effectiveUserID, "remaining_usd", check.RemainingUSD)
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
			http.Error(w, fmt.Sprintf("request translation error: %v", err), http.StatusBadRequest)
			return
		}

		// For translated providers, also check streaming against the translated body.
		isStreaming = isStreamingRequest(providerName, upstreamBody)

		slog.Debug("translated request",
			"provider", providerName,
			"model", translatedModel,
			"streaming", isStreaming)
	}

	// --- Vertex AI body enrichment for native passthrough ---
	// When there's no FormatTranslator (e.g. anthropic-vertex), the body is
	// forwarded as-is. But Vertex AI rawPredict requires:
	//   1. `anthropic_version` in the body (Claude Code sends it as a header)
	//   2. `model` NOT in the body (Vertex AI identifies model from URL path)
	if provider.FormatTranslator == nil && provider.PathRewriter != nil {
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

	// --- Path rewriting ---
	// If the provider has a PathRewriter, rewrite the upstream URL path
	// (e.g. Vertex AI project-scoped model endpoints).
	// When FormatTranslator is nil (e.g. anthropic-vertex), the model comes
	// from the request body rather than from translation.
	modelForPath := translatedModel
	if modelForPath == "" && provider.PathRewriter != nil {
		modelForPath, _ = extractRequestInfo(providerName, reqBody)
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
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Forward headers (auth, content-type, etc).
	forwardHeaders(r, upstreamReq, providerName)

	// Pre-generate the span ID that buildSpan will use for this proxy span.
	// We need it now so the outgoing traceparent to the upstream LLM
	// references the sidecar's span as its parent.
	proxySpanID := generateSpanID()

	// Inject an outgoing traceparent so the upstream LLM API (if it
	// supports OTel) creates spans as children of the sidecar span.
	if traceCtx != nil {
		upstreamReq.Header.Set("Traceparent",
			fmt.Sprintf("00-%s-%s-01", traceCtx.traceID, proxySpanID))
	}

	// --- ADC token injection ---
	// If the provider has a TokenSource, replace the auth header with a fresh token.
	if provider.TokenSource != nil {
		token, tokenErr := provider.TokenSource.Token()
		if tokenErr != nil {
			slog.Error("failed to get ADC token", "provider", providerName, "error", tokenErr)
			http.Error(w, "failed to obtain GCP credentials", http.StatusInternalServerError)
			return
		}
		upstreamReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	// Propagate request ID to upstream.
	upstreamReq.Header.Set("X-Request-ID", requestID)

	// Execute the upstream request.
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		// CRITICAL: Don't leak internal URLs/DNS/network info to the client.
		slog.Error("upstream request failed", "provider", providerName, "error", err)
		http.Error(w, `{"error":{"message":"upstream provider unavailable","type":"upstream_error"}}`, http.StatusBadGateway)
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

	if isStreaming && resp.StatusCode == http.StatusOK {
		p.handleStreamingResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, cbAllow, traceCtx, proxySpanID)
	} else {
		p.handleStandardResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, cbAllow, traceCtx, proxySpanID)
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
) {
	// Read the full response (reject if over 10MB to prevent OOM under concurrent load).
	// CRIT-15: Previously 50MB — with 100 concurrent requests that's 5GB of heap.
	// Matches the 10MB request body limit for symmetry.
	const respLimit = int64(10 << 20)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, respLimit+1))
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}
	if int64(len(respBody)) > respLimit {
		http.Error(w, "upstream response too large", http.StatusBadGateway)
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
	if cbAllow && p.users != nil && effectiveUserID != "" {
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
		inputTokens = p.calc.NormalizeCachedInput(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens)
		deductCtx, deductCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		p.deductBudget(deductCtx, provider, model, effectiveUserID, inputTokens, outputTokens)
		deductCancel()
	}

	// Create observability span (async — don't block the handler).
	// CRIT-16: Use bounded semaphore to prevent goroutine accumulation
	// under Firestore contention (previously unbounded).
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		select {
		case p.spanSem <- struct{}{}:
			go func() {
				defer func() { <-p.spanSem }()
				defer spanCancel()
				p.createSpan(spanCtx, provider, reqBody, respBody, startTime, endTime, resp.StatusCode, ttfb, requestID, sessionID, effectiveUserID, tenantID, jobID, traceCtx, proxySpanID)
			}()
		default:
			spanCancel()
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
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
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
	if cbAllow && p.users != nil && effectiveUserID != "" {
		model, _ := extractRequestInfo(provider.Name, reqBody)
		_, inputTokens, outputTokens := extractStreamingUsage(provider.Name, parseData)
		ct := extractStreamingCacheTokens(provider.Name, parseData)
		if model == "" {
			model = extractModelFromStreamingResponse(provider.Name, parseData)
		}
		inputTokens = p.calc.NormalizeCachedInput(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens)
		deductCtx, deductCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		p.deductBudget(deductCtx, provider, model, effectiveUserID, inputTokens, outputTokens)
		deductCancel()
	}

	// Create observability span (async — don't block the handler).
	// CRIT-16: Use bounded semaphore to prevent goroutine accumulation.
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		select {
		case p.spanSem <- struct{}{}:
			go func() {
				defer func() { <-p.spanSem }()
				defer spanCancel()
				p.createStreamingSpan(spanCtx, provider, reqBody, parseData, startTime, endTime, ttfb, ttft, requestID, sessionID, effectiveUserID, tenantID, jobID, streamStatus, traceCtx, proxySpanID)
			}()
		default:
			spanCancel()
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
	provider        Provider
	model           string
	sessionID       string
	effectiveUserID string // resolved end-user identity (auth email > auth UID)
	tenantID        string // downstream customer/tenant (from Baggage or header)
	jobID           string // experiment/job ID (from Baggage or header)
	inputContent    string
	outputContent   string
	inputTokens     int64
	outputTokens    int64
	cacheTokens     CacheTokens // raw prompt cache breakdown
	startTime       time.Time
	endTime         time.Time
	status          storage.SpanStatus
	ttfb            time.Duration
	requestID       string
	extraAttrs      map[string]string // streaming-specific, status code, etc.
	namePrefix      string            // e.g. "openai.chat" or "openai.chat.stream"
	traceCtx        *traceContext     // W3C trace context from caller (nil = generate new)
	proxySpanID     string            // pre-generated span ID (for outgoing traceparent)
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
		return
	}

	totalTokens := inputTokens + outputTokens
	cost := p.calc.Calculate(provider.Name, model, inputTokens, outputTokens)

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
		slog.Error("deduct_spend: all retries exhausted — spend not recorded",
			"user_id", userID,
			"cost_usd", cost,
			"tokens", totalTokens,
			"attempts", maxDeductAttempts,
			"error", deductErr)
		return
	}

	// Async: threshold notification is best-effort.
	if p.budgetCk != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			budget, err := p.users.GetBudget(bgCtx, userID)
			if err == nil && budget != nil && budget.LimitUSD > 0 {
				var email string
				user, uerr := p.users.GetUser(bgCtx, userID)
				if uerr == nil {
					email = user.Email
				}
				p.budgetCk.CheckAndNotify(bgCtx, userID, email, budget.PeriodKey,
					notify.DeductResult{
						SpentUSD: budget.SpentUSD,
						LimitUSD: budget.LimitUSD,
					})
			}
		}()
	}
}

// buildSpan constructs a storage.Span from parsed parameters and submits it.
// Budget deduction is handled separately by deductBudget (called synchronously
// in the request handler) — buildSpan only handles span creation and storage.
func (p *Proxy) buildSpan(ctx context.Context, params spanParams) {
	totalTokens := params.inputTokens + params.outputTokens
	cost := p.calc.Calculate(params.provider.Name, params.model, params.inputTokens, params.outputTokens)

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
	traceID := generateTraceID()
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
			CacheReadTokens:     params.cacheTokens.CacheReadTokens,
			CacheCreationTokens: params.cacheTokens.CacheCreationTokens,
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
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)
	ct := extractCacheTokens(provider.Name, respBody)

	// For Google native, model is in the URL path, not the body.
	// Fall back to the response's modelVersion field.
	if model == "" {
		model = extractModelFromResponse(provider.Name, respBody)
	}

	// Normalize cached input tokens via the calculator (handles all providers).
	inputTokens = p.calc.NormalizeCachedInput(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens)

	status := storage.SpanStatusOK
	if statusCode >= 400 {
		status = storage.SpanStatusError
	}

	p.buildSpan(ctx, spanParams{
		provider:        provider,
		model:           model,
		effectiveUserID: effectiveUserID,
		tenantID:        tenantID,
		jobID:           jobID,
		inputContent:    inputContent,
		outputContent:   outputContent,
		inputTokens:     inputTokens,
		outputTokens:    outputTokens,
		cacheTokens:     ct,
		startTime:       startTime,
		endTime:         endTime,
		status:          status,
		ttfb:            ttfb,
		requestID:       requestID,
		sessionID:       sessionID,
		namePrefix:      fmt.Sprintf("%s.chat", provider.Name),
		traceCtx:        traceCtx,
		proxySpanID:     proxySpanID,
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
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractStreamingUsage(provider.Name, streamData)
	ct := extractStreamingCacheTokens(provider.Name, streamData)

	// For Google native, model is in the URL path, not the body.
	// Fall back to the response's modelVersion field.
	if model == "" {
		model = extractModelFromStreamingResponse(provider.Name, streamData)
	}

	// Normalize cached input tokens via the calculator (handles all providers).
	inputTokens = p.calc.NormalizeCachedInput(provider.Name, model, inputTokens, ct.CacheReadTokens, ct.CacheCreationTokens)

	p.buildSpan(ctx, spanParams{
		provider:        provider,
		model:           model,
		effectiveUserID: effectiveUserID,
		tenantID:        tenantID,
		jobID:           jobID,
		inputContent:    inputContent,
		outputContent:   outputContent,
		inputTokens:     inputTokens,
		outputTokens:    outputTokens,
		cacheTokens:     ct,
		startTime:       startTime,
		endTime:         endTime,
		status:          streamStatus,
		ttfb:            ttfb,
		requestID:       requestID,
		sessionID:       sessionID,
		namePrefix:      fmt.Sprintf("%s.chat.stream", provider.Name),
		traceCtx:        traceCtx,
		proxySpanID:     proxySpanID,
		extraAttrs: map[string]string{
			"proxy.streaming": "true",
			"llm.ttft_ms":     fmt.Sprintf("%d", ttft.Milliseconds()),
		},
	})
}

// --- Header forwarding ---

func forwardHeaders(src *http.Request, dst *http.Request, provider string) {
	// Always forward these.
	for _, h := range []string{"Authorization", "Content-Type", "Accept"} {
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
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
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
