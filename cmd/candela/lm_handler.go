package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"strings"
	"sync/atomic"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/runtime"
	"golang.org/x/sync/singleflight"
)

// lmHandler implements a smart HTTP handler for the LM Studio compat listener.
// It intercepts /v1/models (merging local + remote + cloud models) and
// /v1/chat/completions (routing to local runtime, cloud proxy, or remote server).
type lmHandler struct {
	mgr              *runtime.Manager       // local runtime manager (may be nil)
	remoteProxy      *httputil.ReverseProxy // proxy to remote Candela server
	localProxy       *httputil.ReverseProxy // proxy to local runtime (e.g. Ollama)
	localHandler     http.Handler           // localProxy wrapped with optional span capture
	cloudProxy       *proxy.Proxy           // direct cloud proxy (solo + cloud mode)
	cloudModels      map[string]string      // model ID → provider name
	calc             *costcalc.Calculator   // pricing calculator (for filtering unpriced models)
	soloMode         bool                   // true = solo mode (use embedded cloud models), false = team mode
	defaultMaxTokens int                    // injected when client omits max_tokens (default: 8192)

	// Model caches stored as atomic.Value holding map[string]bool.
	// Updated atomically by swapping the entire map — no in-place mutation.
	localModels  atomic.Value // map[string]bool (cached for fast routing)
	remoteModels atomic.Value // map[string]bool (cached from remote /v1/models)

	remoteFetchGroup singleflight.Group // deduplicates concurrent lazy fetches

	// Remote model cache TTL: tracks when we last refreshed so periodic
	// callers don't hammer the server on every /v1/models request.
	lastRemoteFetch atomic.Int64 // unix timestamp of last successful fetch

	stopCh chan struct{} // closed by Close() to stop background goroutines
}

// newLMHandler creates a smart LM compat handler that merges local + remote + cloud
// models and routes chat completions to the correct backend.
func newLMHandler(mgr *runtime.Manager, remoteProxy, localProxy *httputil.ReverseProxy, localHandler http.Handler, cloudProxy *proxy.Proxy, cloudModels map[string]string, calc *costcalc.Calculator, soloMode bool, defaultMaxTokens int) *lmHandler {
	if localHandler == nil && localProxy != nil {
		localHandler = localProxy
	}
	if cloudModels == nil {
		cloudModels = make(map[string]string)
	}
	if defaultMaxTokens <= 0 {
		defaultMaxTokens = 8192
	}
	h := &lmHandler{
		mgr:              mgr,
		remoteProxy:      remoteProxy,
		localProxy:       localProxy,
		localHandler:     localHandler,
		cloudProxy:       cloudProxy,
		cloudModels:      cloudModels,
		calc:             calc,
		soloMode:         soloMode,
		defaultMaxTokens: defaultMaxTokens,
		stopCh:           make(chan struct{}),
	}

	// Initialize empty maps in atomic values.
	h.localModels.Store(make(map[string]bool))
	h.remoteModels.Store(make(map[string]bool))

	// In team mode, pre-fetch remote models and start a background refresh.
	if !soloMode && remoteProxy != nil {
		go h.startRemoteRefresh()
	}

	return h
}

// Close stops background goroutines. Safe to call multiple times.
func (h *lmHandler) Close() {
	select {
	case <-h.stopCh:
		// already closed
	default:
		close(h.stopCh)
	}
}

// startRemoteRefresh periodically refreshes the remote model cache.
func (h *lmHandler) startRemoteRefresh() {
	// Initial fetch with a short delay to let the server come up.
	select {
	case <-time.After(2 * time.Second):
	case <-h.stopCh:
		return
	}
	h.refreshRemoteModels()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.refreshRemoteModels()
		case <-h.stopCh:
			return
		}
	}
}

// refreshRemoteModels fetches the model list from the remote server
// and atomically swaps the cache.
// Distinguishes fetch failure (nil → keep stale cache) from a valid
// empty response ([] → clear the cache).
func (h *lmHandler) refreshRemoteModels() {
	req, err := http.NewRequest(http.MethodGet, "/v1/models", nil)
	if err != nil {
		return
	}
	models := h.fetchRemoteModels(req)
	if models == nil {
		return // fetch failed — keep stale cache
	}
	// models may be empty (all disabled) — that's valid, clear the cache.
	newSet := make(map[string]bool, len(models))
	for _, m := range models {
		newSet[m.ID] = true
	}
	h.remoteModels.Store(newSet)
	h.lastRemoteFetch.Store(time.Now().Unix())
	slog.Info("lm handler: remote model cache refreshed", "models", len(models))
}

// loadLocalModels returns the current local model cache.
func (h *lmHandler) loadLocalModels() map[string]bool {
	if m, ok := h.localModels.Load().(map[string]bool); ok {
		return m
	}
	return nil
}

// loadRemoteModels returns the current remote model cache.
func (h *lmHandler) loadRemoteModels() map[string]bool {
	if m, ok := h.remoteModels.Load().(map[string]bool); ok {
		return m
	}
	return nil
}

func (h *lmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	// ── Model discovery ──
	case r.URL.Path == "/v1/models" && r.Method == http.MethodGet:
		h.serveModels(w, r)
	case r.URL.Path == "/api/v0/models" && r.Method == http.MethodGet:
		h.serveModels(w, r)

	// ── Chat completions ──
	case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
		h.serveChat(w, r)
	case r.URL.Path == "/api/v0/chat/completions" && r.Method == http.MethodPost:
		h.serveChat(w, r)
	case r.URL.Path == "/chat/completions" && r.Method == http.MethodPost:
		h.serveChat(w, r)

	// ── LM Studio / OpenAI compat stubs ──
	case r.URL.Path == "/v1/completions" && r.Method == http.MethodPost:
		h.serveCompletionsStub(w, r)
	case r.URL.Path == "/v1/embeddings" && r.Method == http.MethodPost:
		h.serveEmbeddingsStub(w, r)

	// ── LM Studio diagnostics ──
	case r.URL.Path == "/lmstudio/diagnostics" && r.Method == http.MethodGet:
		h.serveDiagnostics(w, r)

	default:
		if h.remoteProxy != nil {
			h.remoteProxy.ServeHTTP(w, r)
		} else {
			proxy.ProxyErrorResponse(w, http.StatusNotFound, "solo mode — no remote server configured", "invalid_request_error")
		}
	}
}

// openaiModel represents a model in the OpenAI /v1/models response.
// Includes LM Studio extension fields required by JetBrains AI Chat.
type openaiModel struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	Created           int64  `json:"created"`
	OwnedBy           string `json:"owned_by"`
	Type              string `json:"type"`               // "llm"
	Publisher         string `json:"publisher"`          // provider name
	Arch              string `json:"arch"`               // "auto" — required by JetBrains
	CompatibilityType string `json:"compatibility_type"` // "gguf"
	State             string `json:"state"`              // "loaded"
	MaxContextLength  int    `json:"max_context_length"` // 128000
}

// openaiModelList is the OpenAI /v1/models response format.
type openaiModelList struct {
	Object string        `json:"object"`
	Data   []openaiModel `json:"data"`
}

// serveModels merges local runtime models with remote and cloud models.
func (h *lmHandler) serveModels(w http.ResponseWriter, r *http.Request) {
	var merged []openaiModel

	// 1. Fetch local models from the runtime.
	if h.mgr != nil && h.localProxy != nil {
		models, err := h.mgr.Runtime().ListModels(r.Context())
		if err != nil {
			slog.Warn("lm handler: failed to list local models", "error", err)
		} else {
			backendName := h.mgr.Runtime().Name()
			// Build new set and atomically swap.
			newSet := make(map[string]bool, len(models))
			for _, m := range models {
				merged = append(merged, openaiModel{
					ID:                m.ID,
					Object:            "model",
					Created:           1700000000, // fixed epoch for stable responses
					OwnedBy:           backendName,
					Type:              "llm",
					Publisher:         backendName,
					Arch:              "auto",
					CompatibilityType: "gguf",
					State:             "loaded",
					MaxContextLength:  128000,
				})
				newSet[m.ID] = true
			}
			h.localModels.Store(newSet)
		}
	}

	// 2. Add direct cloud models (only in solo mode, and only if priced).
	if h.soloMode {
		for modelID, providerName := range h.cloudModels {
			if h.calc != nil && !h.calc.HasPricing(providerName, modelID) {
				slog.Warn("⚠️ hiding cloud model from /v1/models — no pricing configured",
					"model", modelID, "provider", providerName)
				continue
			}
			merged = append(merged, openaiModel{
				ID:                modelID,
				Object:            "model",
				Created:           1700000000, // fixed epoch for stable responses
				OwnedBy:           providerName,
				Type:              "llm",
				Publisher:         providerName,
				Arch:              "auto",
				CompatibilityType: "gguf",
				State:             "loaded",
				MaxContextLength:  128000,
			})
		}
	}

	// 3. Fetch remote models by proxying to the remote Candela server.
	remoteModels := h.fetchRemoteModels(r)
	merged = append(merged, remoteModels...)

	// Cache remote model IDs atomically for alias resolution.
	// Only update if we got a valid response (non-nil), even if empty.
	if remoteModels != nil {
		newRemoteSet := make(map[string]bool, len(remoteModels))
		for _, m := range remoteModels {
			newRemoteSet[m.ID] = true
		}
		h.remoteModels.Store(newRemoteSet)
	}

	// 4. Return merged OpenAI-format response.
	w.Header().Set("Content-Type", "application/json")
	resp := openaiModelList{Object: "list", Data: merged}
	if resp.Data == nil {
		resp.Data = []openaiModel{} // never return null
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// fetchRemoteModels proxies a GET /v1/models to the remote server and parses the response.
func (h *lmHandler) fetchRemoteModels(r *http.Request) []openaiModel {
	if h.remoteProxy == nil {
		return nil // solo mode — no remote
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rec := &responseRecorder{headers: make(http.Header)}
	req := r.Clone(ctx)
	req.URL.Path = "/v1/models" // normalize — remote only serves /v1/models
	h.remoteProxy.ServeHTTP(rec, req)

	if rec.statusCode != http.StatusOK {
		slog.Warn("lm handler: remote /v1/models failed", "status", rec.statusCode)
		return nil
	}

	var resp openaiModelList
	if err := json.Unmarshal(rec.body.Bytes(), &resp); err != nil {
		slog.Warn("lm handler: failed to parse remote models", "error", err)
		return nil
	}
	return resp.Data
}

// serveChat routes chat completions to local runtime, cloud proxy, or remote server.
func (h *lmHandler) serveChat(w http.ResponseWriter, r *http.Request) {
	// Read body to peek at the model field (10MB limit to prevent OOM).
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			proxy.ProxyErrorResponse(w, http.StatusRequestEntityTooLarge, "request body too large", "invalid_request_error")
		} else {
			proxy.ProxyErrorResponse(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		}
		return
	}
	_ = r.Body.Close()

	var req struct {
		Model     string `json:"model"`
		MaxTokens *int   `json:"max_tokens,omitempty"`
		Stream    bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)

	// Normalize path early so all backends (local, cloud, remote) receive
	// the standard /v1/ path regardless of what the client sent.
	r.URL.Path = "/v1/chat/completions"

	// Inject max_tokens if absent or non-positive — some clients (JetBrains)
	// don't send it, but providers like Anthropic require it.
	// Also strip empty tools array (JetBrains sends "tools":[] which causes
	// 422 on strict backends like Mistral) and stream_options (not all
	// upstreams support it).
	{
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err == nil && raw != nil {
			needRewrite := false

			// Inject max_tokens if absent or non-positive.
			if req.MaxTokens == nil || *req.MaxTokens <= 0 {
				defaultMax := h.defaultMaxTokens
				if b, err := json.Marshal(defaultMax); err == nil {
					raw["max_tokens"] = b
					needRewrite = true
				}
			}

			// Strip empty "tools":[] — JetBrains sends it in every request,
			// but Mistral/vLLM reject it with HTTP 422.
			if toolsRaw, ok := raw["tools"]; ok {
				var tools []json.RawMessage
				if json.Unmarshal(toolsRaw, &tools) == nil && len(tools) == 0 {
					delete(raw, "tools")
					delete(raw, "tool_choice")
					needRewrite = true
				}
			}

			// Strip stream_options — not all upstreams support it.
			if _, ok := raw["stream_options"]; ok {
				delete(raw, "stream_options")
				needRewrite = true
			}

			if needRewrite {
				body, _ = json.Marshal(raw)
			}
		}
	}

	// Resolve model aliases (e.g. "claude-sonnet-4" → "claude-sonnet-4-20250514").
	resolved := h.resolveModel(req.Model)
	if resolved != req.Model {
		slog.Info("lm handler: resolved model alias", "from", req.Model, "to", resolved)
		body = rewriteModelInBody(body, req.Model, resolved)
		req.Model = resolved
	}

	// Replay body for the proxy.
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	// 1. Local model → local runtime (with span capture).
	if h.isLocalModel(req.Model) {
		slog.Debug("lm handler: routing to local runtime", "model", req.Model)
		h.localHandler.ServeHTTP(w, r)
		return
	}

	// 2. Cloud model → direct cloud proxy (solo mode only).
	// In team mode, all cloud traffic must go through the remote server
	// so the server catalog remains the single source of truth.
	if h.soloMode {
		if providerName, ok := h.cloudModels[req.Model]; ok && h.cloudProxy != nil {
			slog.Debug("lm handler: routing to cloud provider", "model", req.Model, "provider", providerName)
			r.URL.Path = fmt.Sprintf("/proxy/%s/v1/chat/completions", providerName)
			// Use the full SSE normalizer for solo cloud routes — raw provider
			// responses need chunk normalization (Claude role-only deltas,
			// Qwen content:null, empty choices, etc.).
			writer := w
			if req.Stream {
				writer = newSSENormalizer(w)
			}
			h.cloudProxy.ServeHTTP(writer, r)
			return
		}
	}

	// 3. Remote server → team mode proxy.
	if h.remoteProxy != nil {
		// The remote Candela server already returns clean OpenAI-compatible
		// SSE — no chunk normalization needed. Only apply lightweight header
		// fixes (Content-Type charset, Content-Length) that ktor requires.
		writer := w
		if req.Stream {
			writer = newSSEHeaderNormalizer(w)
		}
		h.remoteProxy.ServeHTTP(writer, r)
		return
	}

	// 4. No handler found.
	proxy.ProxyErrorResponse(w, http.StatusNotFound, "model not found locally and no remote server configured", "invalid_request_error")
}

// isLocalModel checks if a model is served by the local runtime.
func (h *lmHandler) isLocalModel(model string) bool {
	if model == "" || h.mgr == nil || h.localProxy == nil {
		return false
	}
	locals := h.loadLocalModels()
	if locals[model] {
		return true
	}
	// Also check without tag (e.g., "llama3.2" → "llama3.2:latest").
	if !strings.Contains(model, ":") {
		return locals[model+":latest"]
	}
	return false
}

// resolveModel resolves a model name to its canonical ID using prefix matching.
// For example, "claude-sonnet-4" resolves to "claude-sonnet-4-20250514" if that
// is the only model with that prefix. If zero or multiple models match, the
// original name is returned unchanged.
func (h *lmHandler) resolveModel(model string) string {
	if model == "" {
		return model
	}

	// Collect all known model IDs from atomic caches.
	var allModels []string
	for id := range h.loadLocalModels() {
		allModels = append(allModels, id)
	}
	for id := range h.cloudModels {
		allModels = append(allModels, id)
	}

	// Lazy-populate remote models if cache is empty and we have a remote proxy.
	// Uses singleflight to prevent thundering herd on concurrent first requests.
	remoteCache := h.loadRemoteModels()
	if len(remoteCache) == 0 && h.remoteProxy != nil {
		_, _, _ = h.remoteFetchGroup.Do("fetch-remote-models", func() (any, error) {
			req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
			if req != nil {
				remote := h.fetchRemoteModels(req)
				if len(remote) > 0 {
					newSet := make(map[string]bool, len(remote))
					for _, m := range remote {
						newSet[m.ID] = true
					}
					h.remoteModels.Store(newSet)
				}
			}
			return nil, nil
		})
		// Re-read after potential update.
		remoteCache = h.loadRemoteModels()
	}

	for id := range remoteCache {
		allModels = append(allModels, id)
	}

	// Exact match → no resolution needed.
	for _, id := range allModels {
		if id == model {
			return model
		}
	}

	// Prefix match → resolve if exactly one model matches.
	var matches []string
	for _, id := range allModels {
		if strings.HasPrefix(id, model) {
			matches = append(matches, id)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}

	// Ambiguous or no match — return original.
	return model
}

// rewriteModelInBody replaces the "model" field value in a JSON request body.
// It parses the body to find the current model string, then locates the
// "model" key via bytes.Index and splices in the new JSON-encoded value.
// This avoids per-request regex compilation while still handling whitespace
// around the colon and replacing only the first matching occurrence.
func rewriteModelInBody(body []byte, _, newModel string) []byte {
	// Parse to get the ground-truth model string (handles all JSON encodings).
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		return body
	}
	oldJSON, _ := json.Marshal(req.Model)
	newJSON, _ := json.Marshal(newModel)

	// Scan through all occurrences of the "model" key. The word "model" may
	// appear inside user message content before the actual JSON key, so we
	// must verify each hit has a colon + matching value before splicing.
	key := []byte(`"model"`)
	offset := 0
	for {
		idx := bytes.Index(body[offset:], key)
		if idx < 0 {
			return body // no more occurrences
		}
		idx += offset // absolute position

		// Skip whitespace and colon after the key.
		afterKey := body[idx+len(key):]
		valueStart := 0
		for valueStart < len(afterKey) && (afterKey[valueStart] == ' ' || afterKey[valueStart] == '\t' || afterKey[valueStart] == '\n' || afterKey[valueStart] == '\r' || afterKey[valueStart] == ':') {
			valueStart++
		}
		// Check if the value at this position matches the old JSON-encoded model.
		if valueStart+len(oldJSON) <= len(afterKey) && bytes.Equal(afterKey[valueStart:valueStart+len(oldJSON)], oldJSON) {
			// Found it — splice.
			absValueStart := idx + len(key) + valueStart
			result := make([]byte, 0, len(body)+len(newJSON)-len(oldJSON))
			result = append(result, body[:absValueStart]...)
			result = append(result, newJSON...)
			result = append(result, body[absValueStart+len(oldJSON):]...)
			return result
		}
		// Move past this occurrence and keep searching.
		offset = idx + len(key)
	}
}

// responseRecorder captures a proxy response for parsing.
// Buffer is capped at maxRecorderBytes to prevent OOM on large responses.
type responseRecorder struct {
	headers    http.Header
	body       bytes.Buffer
	statusCode int
	capped     bool
}

const maxRecorderBytes = 2 << 20 // 2MB — enough for /v1/models JSON

func (r *responseRecorder) Header() http.Header { return r.headers }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK // match net/http implicit behavior
	}
	if !r.capped {
		if r.body.Len()+len(b) > maxRecorderBytes {
			remaining := maxRecorderBytes - r.body.Len()
			if remaining > 0 {
				r.body.Write(b[:remaining])
			}
			r.capped = true
		} else {
			r.body.Write(b)
		}
	}
	return len(b), nil
}
func (r *responseRecorder) WriteHeader(code int) { r.statusCode = code }

// sseHeadersToStrip lists upstream provider headers that LM Studio wouldn't
// send. Shared between sseHeaderNormalizer and sseNormalizer to keep the
// header cleanup policy in sync.
var sseHeadersToStrip = []string{
	"Server", "Alt-Svc", "Via",
	"X-Frame-Options", "X-Xss-Protection", "X-Accel-Buffering",
	"X-Vertex-Ai-Received-Request-Id", "Request-Id",
}

// normalizeSSEHeaders fixes SSE response headers for ktor compatibility:
// strips Content-Type charset, removes Content-Length, and cleans provider headers.
func normalizeSSEHeaders(header http.Header) {
	ct := header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		return
	}
	if ct != "text/event-stream" {
		header.Set("Content-Type", "text/event-stream")
	}
	header.Del("Content-Length")
	for _, h := range sseHeadersToStrip {
		header.Del(h)
	}
}

// sseHeaderNormalizer wraps an http.ResponseWriter to fix SSE response headers
// without modifying the event stream. Used for remote proxy traffic where the
// upstream already returns clean OpenAI-compatible chunks.
type sseHeaderNormalizer struct {
	w           http.ResponseWriter
	header      http.Header
	wroteHeader bool
}

func newSSEHeaderNormalizer(w http.ResponseWriter) *sseHeaderNormalizer {
	return &sseHeaderNormalizer{w: w, header: w.Header()}
}

func (n *sseHeaderNormalizer) Header() http.Header { return n.header }

func (n *sseHeaderNormalizer) WriteHeader(code int) {
	if n.wroteHeader {
		return
	}
	n.wroteHeader = true
	normalizeSSEHeaders(n.header)
	n.w.WriteHeader(code)
}

func (n *sseHeaderNormalizer) Write(b []byte) (int, error) {
	if !n.wroteHeader {
		n.WriteHeader(http.StatusOK)
	}
	return n.w.Write(b) // pass through unchanged
}

func (n *sseHeaderNormalizer) Flush() {
	if f, ok := n.w.(http.Flusher); ok {
		f.Flush()
	}
}

// sseNormalizer wraps an http.ResponseWriter to intercept SSE events and
// normalize them for JetBrains compatibility. It filters out chunks with
// empty choices arrays (Qwen) and ensures delta.content is always present
// (Claude sends chunks with only delta.role).
type sseNormalizer struct {
	w      http.ResponseWriter
	buf    []byte // accumulates partial writes until we have a complete SSE event
	header http.Header
}

func newSSENormalizer(w http.ResponseWriter) *sseNormalizer {
	return &sseNormalizer{w: w, header: w.Header()}
}

func (n *sseNormalizer) Header() http.Header { return n.header }

func (n *sseNormalizer) WriteHeader(code int) {
	normalizeSSEHeaders(n.header)
	n.w.WriteHeader(code)
}

func (n *sseNormalizer) Flush() {
	if f, ok := n.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (n *sseNormalizer) Write(b []byte) (int, error) {
	originalLen := len(b)
	n.buf = append(n.buf, b...)

	// Process complete SSE events (delimited by \n\n).
	// Batch all events from this Write call into a single output write
	// to minimize the number of HTTP/1.1 chunked frames.
	var out bytes.Buffer
	for {
		idx := bytes.Index(n.buf, []byte("\n\n"))
		if idx == -1 {
			break
		}
		event := n.buf[:idx+2] // include the \n\n
		n.buf = n.buf[idx+2:]

		normalized := n.normalizeEvent(event)
		if normalized != nil {
			out.Write(normalized)
		}
	}
	if out.Len() > 0 {
		if _, err := n.w.Write(out.Bytes()); err != nil {
			return originalLen, err
		}
		n.Flush()
	}
	return originalLen, nil
}

// normalizeEvent processes a single SSE event. Returns nil to suppress the event.
func (n *sseNormalizer) normalizeEvent(event []byte) []byte {
	line := bytes.TrimSpace(event)

	// Pass through non-data lines (comments, empty lines, "data: [DONE]").
	if !bytes.HasPrefix(line, []byte("data: ")) {
		return event
	}
	payload := bytes.TrimPrefix(line, []byte("data: "))

	// Pass through [DONE] sentinel.
	if bytes.Equal(payload, []byte("[DONE]")) {
		return event
	}

	// Parse the JSON chunk.
	var chunk map[string]json.RawMessage
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return event // pass through unparseable events
	}

	// Parse choices array.
	choicesRaw, ok := chunk["choices"]
	if !ok {
		return event
	}

	var choices []map[string]json.RawMessage
	if err := json.Unmarshal(choicesRaw, &choices); err != nil {
		return event
	}

	// Drop events with empty choices array (Qwen final usage-only chunk).
	if len(choices) == 0 {
		return nil
	}

	modified := false

	for i := range choices {
		deltaRaw, hasDelta := choices[i]["delta"]
		if !hasDelta {
			continue
		}

		var delta map[string]json.RawMessage
		if err := json.Unmarshal(deltaRaw, &delta); err != nil {
			continue
		}

		// LM Studio spec: final chunk should have empty delta {} with
		// finish_reason "stop". Normalize providers that include content
		// in the final chunk.
		if frRaw, hasFR := choices[i]["finish_reason"]; hasFR {
			var fr string
			if json.Unmarshal(frRaw, &fr) == nil && fr == "stop" {
				// Clear the delta to match LM Studio's format.
				for k := range delta {
					delete(delta, k)
				}
				modified = true
				if b, err := json.Marshal(delta); err == nil {
					choices[i]["delta"] = b
				}
				// Remove non-standard choice-level fields before continuing.
				for _, field := range []string{"matched_stop"} {
					delete(choices[i], field)
				}
				continue
			}
		}

		// Ensure delta.content is always a string (Claude sends role-only
		// first chunk; Qwen sends content:null on its finish chunk).
		contentRaw, hasContent := delta["content"]
		if !hasContent || bytes.Equal(contentRaw, []byte("null")) {
			delta["content"] = json.RawMessage(`""`)
			modified = true
		}

		// Remove non-standard fields that may confuse strict parsers.
		for _, field := range []string{"reasoning_content", "tool_calls", "matched_stop"} {
			if _, has := delta[field]; has {
				delete(delta, field)
				modified = true
			}
		}

		// Remove non-standard choice-level fields.
		for _, field := range []string{"matched_stop"} {
			if _, has := choices[i][field]; has {
				delete(choices[i], field)
				modified = true
			}
		}

		if modified {
			if b, err := json.Marshal(delta); err == nil {
				choices[i]["delta"] = b
			}
		}
	}

	if !modified {
		return event
	}

	// Re-serialize.
	if b, err := json.Marshal(choices); err == nil {
		chunk["choices"] = b
	}
	if b, err := json.Marshal(chunk); err == nil {
		var out bytes.Buffer
		out.WriteString("data: ")
		out.Write(b)
		out.WriteString("\n\n")
		return out.Bytes()
	}

	return event // fallback: pass through
}

// ── LM Studio compat endpoint stubs ──

// serveCompletionsStub returns 501 for the legacy /v1/completions endpoint.
// Candela is a chat-completions proxy and does not support the legacy
// completions API. This prevents confusing 404s.
func (h *lmHandler) serveCompletionsStub(w http.ResponseWriter, _ *http.Request) {
	proxy.ProxyErrorResponse(w, http.StatusNotImplemented,
		"legacy /v1/completions is not supported — use /v1/chat/completions",
		"invalid_request_error")
}

// serveEmbeddingsStub returns 501 for the /v1/embeddings endpoint.
// Candela does not currently support embedding generation. This prevents
// confusing 404s when clients probe for capabilities.
func (h *lmHandler) serveEmbeddingsStub(w http.ResponseWriter, _ *http.Request) {
	proxy.ProxyErrorResponse(w, http.StatusNotImplemented,
		"/v1/embeddings is not currently supported",
		"invalid_request_error")
}

// serveDiagnostics returns a lightweight JSON health check compatible with
// LM Studio's /lmstudio/diagnostics endpoint. JetBrains and other clients
// may use this to verify the server is alive before sending requests.
func (h *lmHandler) serveDiagnostics(w http.ResponseWriter, _ *http.Request) {
	localCount := 0
	if m := h.loadLocalModels(); m != nil {
		localCount = len(m)
	}
	remoteCount := 0
	if m := h.loadRemoteModels(); m != nil {
		remoteCount = len(m)
	}
	cloudCount := len(h.cloudModels)

	runtimeBackend := "none"
	if h.mgr != nil {
		runtimeBackend = h.mgr.Runtime().Name()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"version": "candela",
		"models": map[string]int{
			"local":  localCount,
			"remote": remoteCount,
			"cloud":  cloudCount,
			"total":  localCount + remoteCount + cloudCount,
		},
		"runtime": runtimeBackend,
	})
}
