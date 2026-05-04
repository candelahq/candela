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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"log/slog"

	"golang.org/x/oauth2"

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
}

// Proxy handles LLM API proxying with observability.
type Proxy struct {
	providers map[string]Provider
	submitter SpanSubmitter
	calc      *costcalc.Calculator
	client    *http.Client
	projectID string
	breakers  map[string]*CircuitBreaker

	// Optional dependencies for team-mode features.
	users    storage.UserStore     // Budget deduction (nil = no budget tracking)
	budgetCk *notify.BudgetChecker // Budget threshold notifications (nil = no alerts)
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

// requestIDPattern validates that a request ID contains only safe characters
// (alphanumeric, hyphens) and is between 1-128 chars.
// This prevents log injection and trace poisoning via crafted X-Request-ID headers.
var requestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,128}$`)

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Generate or accept request ID.
	// CRITICAL: Validate to prevent log/trace injection.
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" || !requestIDPattern.MatchString(requestID) {
		requestID = generateTraceID() // 32-char hex
	}

	// Accept session ID from candela-local (or other clients).
	sessionID := r.Header.Get("X-Session-Id")

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
		if caller.Email != "" {
			effectiveUserID = strings.ToLower(caller.Email)
			isServiceAccount = strings.HasSuffix(effectiveUserID, ".gserviceaccount.com")
		} else {
			effectiveUserID = caller.ID
		}
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
	// Soft gate: check if the user has *any* remaining budget (grants + base).
	// We pass estimatedCostUSD=0 because we can't know the actual cost until
	// after the upstream response. This blocks only fully-exhausted users;
	// actual cost deduction happens post-response via DeductSpend.
	// Uses effectiveUserID so budget is checked against the real end-user.
	// Service accounts have no budget entries — skip to avoid spurious warnings.
	if p.users != nil && effectiveUserID != "" && !isServiceAccount {
		// ── Rate limiting ──
		// Enforce per-user request velocity limits before budget check.
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

		// ── Pre-flight budget check ──
		check, err := p.users.CheckBudget(r.Context(), effectiveUserID, 0)
		if err != nil {
			slog.Warn("budget check failed, allowing request",
				"user_id", effectiveUserID, "error", err)
		} else if !check.Allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"message":"budget exhausted — contact your admin for a grant or budget increase","type":"insufficient_budget","code":402}}`))
			slog.Info("blocked request: budget exhausted",
				"user_id", effectiveUserID,
				"remaining_usd", check.RemainingUSD)
			return
		}
	}

	// Handle GET /v1/models — return synthetic model list for OpenAI-compatible clients.
	if r.Method == http.MethodGet && strings.HasSuffix(upstreamPath, "/models") {
		w.Header().Set("Content-Type", "application/json")
		modelsResp := `{"object":"list","data":[{"id":"claude-sonnet-4-20250514","object":"model","created":1700000000,"owned_by":"anthropic"}]}`
		_, _ = w.Write([]byte(modelsResp))
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

	// --- Translation layer ---
	// If the provider has a FormatTranslator, convert the request format
	// (e.g. OpenAI Chat Completions → Anthropic Messages).
	var translatedModel string
	upstreamBody := reqBody
	if provider.FormatTranslator != nil {
		upstreamBody, translatedModel, err = provider.FormatTranslator.TranslateRequest(reqBody)
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

	// --- Path rewriting ---
	// If the provider has a PathRewriter, rewrite the upstream URL path
	// (e.g. Vertex AI project-scoped model endpoints).
	if provider.PathRewriter != nil && translatedModel != "" {
		upstreamPath = provider.PathRewriter.RewritePath(translatedModel, isStreaming)
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

	if isStreaming && resp.StatusCode == http.StatusOK {
		p.handleStreamingResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, cbAllow, traceCtx, proxySpanID)
	} else {
		p.handleStandardResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, effectiveUserID, cbAllow, traceCtx, proxySpanID)
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
	cbAllow bool,
	traceCtx *traceContext,
	proxySpanID string,
) {
	// Read the full response (reject if over 50MB to prevent OOM/truncation).
	const respLimit = int64(50 << 20)
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

	// Create observability span (async — don't block the response).
	// Use a bounded timeout (not WithoutCancel) to prevent goroutine leaks
	// if downstream services (Firestore, storage) hang.
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer spanCancel()
			p.createSpan(spanCtx, provider, reqBody, respBody, startTime, endTime, resp.StatusCode, ttfb, requestID, sessionID, effectiveUserID, traceCtx, proxySpanID)
		}()
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
	// Buffer capped at 1MB to prevent OOM on large streaming responses.
	var streamBuffer bytes.Buffer
	const maxStreamCapture = 1 << 20 // 1MB
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
					remaining := maxStreamCapture - streamBuffer.Len()
					if remaining > 0 {
						streamBuffer.Write(chunk[:remaining])
					}
					streamCapped = true
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

	// Parse the accumulated stream to extract usage data.
	// Use bounded timeout to prevent goroutine leaks.
	streamStatus := storage.SpanStatusOK
	if !streamCompleted {
		streamStatus = storage.SpanStatusError
	}
	if cbAllow {
		spanCtx, spanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer spanCancel()
			p.createStreamingSpan(spanCtx, provider, reqBody, streamBuffer.Bytes(), startTime, endTime, ttfb, ttft, requestID, sessionID, effectiveUserID, streamStatus, traceCtx, proxySpanID)
		}()
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
	inputContent    string
	outputContent   string
	inputTokens     int64
	outputTokens    int64
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

// buildSpan constructs a storage.Span from parsed parameters and submits it.
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
			Model:         params.model,
			Provider:      params.provider.Name,
			InputTokens:   params.inputTokens,
			OutputTokens:  params.outputTokens,
			TotalTokens:   totalTokens,
			CostUSD:       cost,
			InputContent:  truncate(params.inputContent, 10000),
			OutputContent: truncate(params.outputContent, 10000),
		},
		Attributes: attrs,
	}

	// Set user ID for per-user attribution (resolved once in handleProxy).
	span.UserID = params.effectiveUserID

	// Set session ID for conversation grouping.
	span.SessionID = params.sessionID

	if p.submitter != nil {
		p.submitter.SubmitBatch([]storage.Span{span})
	}

	// Budget deduction — SYNCHRONOUS to prevent billing bypass on crash.
	// Notification is non-critical and remains async.
	// Skip for service accounts — they have no budget entries.
	isSA := strings.HasSuffix(span.UserID, ".gserviceaccount.com")
	if p.users != nil && span.UserID != "" && !isSA && span.GenAI != nil && span.GenAI.CostUSD > 0 {
		if err := p.users.DeductSpend(ctx, span.UserID, span.GenAI.CostUSD, span.GenAI.TotalTokens); err != nil {
			slog.Error("failed to deduct spend",
				"user_id", span.UserID,
				"cost_usd", span.GenAI.CostUSD,
				"error", err)
		} else if p.budgetCk != nil {
			// Async: threshold notification is best-effort.
			go func() {
				bgCtx := context.Background()
				budget, err := p.users.GetBudget(bgCtx, span.UserID)
				if err == nil && budget != nil && budget.LimitUSD > 0 {
					var email string
					user, uerr := p.users.GetUser(bgCtx, span.UserID)
					if uerr == nil {
						email = user.Email
					}
					p.budgetCk.CheckAndNotify(bgCtx, span.UserID, email, budget.PeriodKey,
						notify.DeductResult{
							SpentUSD: budget.SpentUSD,
							LimitUSD: budget.LimitUSD,
						})
				}
			}()
		}
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
	traceCtx *traceContext,
	proxySpanID string,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)

	status := storage.SpanStatusOK
	if statusCode >= 400 {
		status = storage.SpanStatusError
	}

	p.buildSpan(ctx, spanParams{
		provider:        provider,
		model:           model,
		effectiveUserID: effectiveUserID,
		inputContent:    inputContent,
		outputContent:   outputContent,
		inputTokens:     inputTokens,
		outputTokens:    outputTokens,
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
	streamStatus storage.SpanStatus,
	traceCtx *traceContext,
	proxySpanID string,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractStreamingUsage(provider.Name, streamData)

	p.buildSpan(ctx, spanParams{
		provider:        provider,
		model:           model,
		effectiveUserID: effectiveUserID,
		inputContent:    inputContent,
		outputContent:   outputContent,
		inputTokens:     inputTokens,
		outputTokens:    outputTokens,
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
		for _, h := range []string{"X-Api-Key", "Anthropic-Version"} {
			if v := src.Header.Get(h); v != "" {
				dst.Header.Set(h, v)
			}
		}
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Operate on runes to avoid cutting multi-byte UTF-8 characters mid-sequence,
	// which would produce invalid strings rejected by BigQuery and protobuf.
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "...[truncated]"
}
