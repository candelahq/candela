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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
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
			_, _ = fmt.Fprintf(w, `{"error":{"message":"unknown model: %s. Configure it in proxy.lmstudio.models","type":"invalid_request_error"}}`, req.Model)
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

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Generate or accept request ID.
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateSpanID() + generateSpanID() // 32-char hex
	}

	// Accept session ID from candela-local (or other clients).
	sessionID := r.Header.Get("X-Session-Id")

	// Accept end-user identity from candela-local. When the proxy is called
	// via a service account, X-User-Id carries the real user's email so
	// spans are attributed correctly for per-user dashboards.
	//
	// Security: only trust X-User-Id from service account callers
	// (*.iam.gserviceaccount.com). Regular users cannot impersonate others.
	caller := auth.FromContext(r.Context())
	var userOverride string
	if xuid := r.Header.Get("X-User-Id"); xuid != "" {
		if caller != nil && strings.HasSuffix(caller.Email, ".iam.gserviceaccount.com") {
			userOverride = strings.ToLower(xuid)
		} else {
			slog.Warn("X-User-Id header ignored — caller is not a service account",
				"caller_email", auth.EmailFromContext(r.Context()), "attempted_override", xuid)
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
		http.Error(w, fmt.Sprintf("unknown provider: %s", providerName), http.StatusBadRequest)
		return
	}

	// ── Pre-flight budget check ──
	// Soft gate: check if the user has *any* remaining budget (grants + base).
	// We pass estimatedCostUSD=0 because we can't know the actual cost until
	// after the upstream response. This blocks only fully-exhausted users;
	// actual cost deduction happens post-response via DeductSpend.
	if p.users != nil {
		// Use userOverride (from X-User-Id) when present, so candela-local
		// users get budget-checked against *their* identity, not the SA's.
		budgetUserID := userOverride
		if budgetUserID == "" && caller != nil {
			budgetUserID = caller.Email
			if budgetUserID == "" {
				budgetUserID = caller.ID
			}
		}
		if budgetUserID != "" {
			check, err := p.users.CheckBudget(r.Context(), budgetUserID, 0)
			if err != nil {
				slog.Warn("budget check failed, allowing request",
					"user_id", budgetUserID, "error", err)
			} else if !check.Allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				_, _ = w.Write([]byte(`{"error":{"message":"budget exhausted — contact your admin for a grant or budget increase","type":"insufficient_budget","code":402}}`))
				slog.Info("blocked request: budget exhausted",
					"user_id", budgetUserID,
					"remaining_usd", check.RemainingUSD)
				return
			}
		}
	}

	// Handle GET /v1/models — return synthetic model list for OpenAI-compatible clients.
	if r.Method == http.MethodGet && strings.HasSuffix(upstreamPath, "/models") {
		w.Header().Set("Content-Type", "application/json")
		modelsResp := `{"object":"list","data":[{"id":"claude-sonnet-4-20250514","object":"model","created":1700000000,"owned_by":"anthropic"}]}`
		_, _ = w.Write([]byte(modelsResp))
		return
	}

	// Read the request body.
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
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
		p.handleStreamingResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, userOverride, cbAllow)
	} else {
		p.handleStandardResponse(w, r, resp, provider, reqBody, startTime, ttfb, requestID, sessionID, userOverride, cbAllow)
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
	userOverride string,
	cbAllow bool,
) {
	// Read the full response.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
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
	if cbAllow {
		go p.createSpan(context.WithoutCancel(r.Context()), provider, reqBody, respBody, startTime, endTime, resp.StatusCode, ttfb, requestID, sessionID, userOverride)
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
	userOverride string,
	cbAllow bool,
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
	var streamBuffer bytes.Buffer
	buf := make([]byte, 4096)
	var ttft time.Duration
	isFirstChunk := true

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if isFirstChunk {
				ttft = time.Since(startTime)
				isFirstChunk = false
			}

			chunk := buf[:n]

			// Buffer raw upstream data for observability (before translation).
			if cbAllow {
				streamBuffer.Write(chunk)
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
			break
		}
	}

	endTime := time.Now()

	// Parse the accumulated stream to extract usage data.
	if cbAllow {
		go p.createStreamingSpan(context.WithoutCancel(r.Context()), provider, reqBody, streamBuffer.Bytes(), startTime, endTime, ttfb, ttft, requestID, sessionID, userOverride)
	} else {
		slog.Debug("skipping streaming span (circuit open)", "provider", provider.Name, "request_id", requestID)
	}
}

// spanParams holds the parsed data needed to build an observability span.
// Both standard and streaming responses produce the same struct; the only
// difference is how they parse the upstream response.
type spanParams struct {
	provider      Provider
	model         string
	sessionID     string
	userOverride  string // end-user email from X-User-Id header (candela-local)
	inputContent  string
	outputContent string
	inputTokens   int64
	outputTokens  int64
	startTime     time.Time
	endTime       time.Time
	status        storage.SpanStatus
	ttfb          time.Duration
	requestID     string
	extraAttrs    map[string]string // streaming-specific, status code, etc.
	namePrefix    string            // e.g. "openai.chat" or "openai.chat.stream"
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

	span := storage.Span{
		SpanID:    generateSpanID(),
		TraceID:   generateTraceID(),
		Name:      params.namePrefix,
		Kind:      storage.SpanKindLLM,
		Status:    params.status,
		StartTime: params.startTime,
		EndTime:   params.endTime,
		Duration:  params.endTime.Sub(params.startTime),
		ProjectID: p.projectID,
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

	// Set user ID for per-user attribution.
	// Priority: (1) X-User-Id header from candela-local (end-user email),
	// (2) auth context email, (3) auth context UID.
	if params.userOverride != "" {
		span.UserID = params.userOverride
	} else if caller := auth.FromContext(ctx); caller != nil {
		if caller.Email != "" {
			span.UserID = strings.ToLower(caller.Email)
		} else {
			span.UserID = caller.ID
		}
	}

	// Set session ID for conversation grouping.
	span.SessionID = params.sessionID

	if p.submitter != nil {
		p.submitter.SubmitBatch([]storage.Span{span})
	}

	// Async budget deduction and notification.
	if p.users != nil && span.UserID != "" && span.GenAI != nil && span.GenAI.CostUSD > 0 {
		go func() {
			ctx := context.Background()
			if err := p.users.DeductSpend(ctx, span.UserID, span.GenAI.CostUSD, span.GenAI.TotalTokens); err != nil {
				slog.Error("failed to deduct spend",
					"user_id", span.UserID,
					"cost_usd", span.GenAI.CostUSD,
					"error", err)
				return
			}

			// Check budget thresholds and notify if needed.
			if p.budgetCk != nil {
				budget, err := p.users.GetBudget(ctx, span.UserID)
				if err == nil && budget != nil && budget.LimitUSD > 0 {
					var email string
					user, uerr := p.users.GetUser(ctx, span.UserID)
					if uerr == nil {
						email = user.Email
					}
					p.budgetCk.CheckAndNotify(ctx, span.UserID, email, budget.PeriodKey,
						notify.DeductResult{
							SpentUSD: budget.SpentUSD,
							LimitUSD: budget.LimitUSD,
						})
				}
			}
		}()
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
	userOverride string,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)

	status := storage.SpanStatusOK
	if statusCode >= 400 {
		status = storage.SpanStatusError
	}

	p.buildSpan(ctx, spanParams{
		provider:      provider,
		model:         model,
		userOverride:  userOverride,
		inputContent:  inputContent,
		outputContent: outputContent,
		inputTokens:   inputTokens,
		outputTokens:  outputTokens,
		startTime:     startTime,
		endTime:       endTime,
		status:        status,
		ttfb:          ttfb,
		requestID:     requestID,
		sessionID:     sessionID,
		namePrefix:    fmt.Sprintf("%s.chat", provider.Name),
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
	userOverride string,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractStreamingUsage(provider.Name, streamData)

	p.buildSpan(ctx, spanParams{
		provider:      provider,
		model:         model,
		userOverride:  userOverride,
		inputContent:  inputContent,
		outputContent: outputContent,
		inputTokens:   inputTokens,
		outputTokens:  outputTokens,
		startTime:     startTime,
		endTime:       endTime,
		status:        storage.SpanStatusOK,
		ttfb:          ttfb,
		requestID:     requestID,
		sessionID:     sessionID,
		namePrefix:    fmt.Sprintf("%s.chat.stream", provider.Name),
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
	return s[:maxLen] + "...[truncated]"
}
