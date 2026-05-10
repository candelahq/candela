package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
	"sync"
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
	mgr          *runtime.Manager       // local runtime manager (may be nil)
	remoteProxy  *httputil.ReverseProxy // proxy to remote Candela server
	localProxy   *httputil.ReverseProxy // proxy to local runtime (e.g. Ollama)
	localHandler http.Handler           // localProxy wrapped with optional span capture
	cloudProxy   *proxy.Proxy           // direct cloud proxy (solo + cloud mode)
	cloudModels  map[string]string      // model ID → provider name
	calc         *costcalc.Calculator   // pricing calculator (for filtering unpriced models)

	localModels  sync.Map // model ID string → bool (cached for fast routing)
	remoteModels sync.Map // model ID string → bool (cached from remote /v1/models)

	remoteFetchGroup singleflight.Group // deduplicates concurrent lazy fetches
}

// newLMHandler creates a smart LM compat handler that merges local + remote + cloud
// models and routes chat completions to the correct backend.
func newLMHandler(mgr *runtime.Manager, remoteProxy, localProxy *httputil.ReverseProxy, localHandler http.Handler, cloudProxy *proxy.Proxy, cloudModels map[string]string, calc *costcalc.Calculator) *lmHandler {
	if localHandler == nil && localProxy != nil {
		localHandler = localProxy
	}
	if cloudModels == nil {
		cloudModels = make(map[string]string)
	}
	return &lmHandler{
		mgr:          mgr,
		remoteProxy:  remoteProxy,
		localProxy:   localProxy,
		localHandler: localHandler,
		cloudProxy:   cloudProxy,
		cloudModels:  cloudModels,
		calc:         calc,
	}
}

func (h *lmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/models" && r.Method == http.MethodGet:
		h.serveModels(w, r)
	case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
		h.serveChat(w, r)
	default:
		if h.remoteProxy != nil {
			h.remoteProxy.ServeHTTP(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "solo mode — no remote server configured"})
		}
	}
}

// openaiModel represents a model in the OpenAI /v1/models response.
type openaiModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
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
			// Refresh the cached model set.
			newSet := make(map[string]bool, len(models))
			for _, m := range models {
				merged = append(merged, openaiModel{
					ID:      m.ID,
					Object:  "model",
					OwnedBy: backendName,
				})
				newSet[m.ID] = true
				h.localModels.Store(m.ID, true)
			}
			// Remove stale entries.
			h.localModels.Range(func(key, _ any) bool {
				if !newSet[key.(string)] {
					h.localModels.Delete(key)
				}
				return true
			})
		}
	}

	// 2. Add direct cloud models (only if priced).
	for modelID, providerName := range h.cloudModels {
		if h.calc != nil && !h.calc.HasPricing(providerName, modelID) {
			slog.Warn("⚠️ hiding cloud model from /v1/models — no pricing configured",
				"model", modelID, "provider", providerName)
			continue
		}
		merged = append(merged, openaiModel{
			ID:      modelID,
			Object:  "model",
			OwnedBy: providerName,
		})
	}

	// 3. Fetch remote models by proxying to the remote Candela server.
	remoteModels := h.fetchRemoteModels(r)
	merged = append(merged, remoteModels...)

	// Cache remote model IDs for alias resolution.
	newRemoteSet := make(map[string]bool, len(remoteModels))
	for _, m := range remoteModels {
		newRemoteSet[m.ID] = true
		h.remoteModels.Store(m.ID, true)
	}
	// Remove stale remote entries.
	h.remoteModels.Range(func(key, _ any) bool {
		if !newRemoteSet[key.(string)] {
			h.remoteModels.Delete(key)
		}
		return true
	})

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
		http.Error(w, `{"error":"request body too large or unreadable"}`, http.StatusRequestEntityTooLarge)
		return
	}
	_ = r.Body.Close()

	var req struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &req)

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

	// 2. Cloud model → direct cloud proxy (solo + cloud mode).
	if providerName, ok := h.cloudModels[req.Model]; ok && h.cloudProxy != nil {
		slog.Debug("lm handler: routing to cloud provider", "model", req.Model, "provider", providerName)
		// Rewrite path for the proxy package: /proxy/{provider}/v1/chat/completions
		r.URL.Path = fmt.Sprintf("/proxy/%s/v1/chat/completions", providerName)
		h.cloudProxy.ServeHTTP(w, r)
		return
	}

	// 3. Remote server → team mode proxy.
	if h.remoteProxy != nil {
		h.remoteProxy.ServeHTTP(w, r)
		return
	}

	// 4. No handler found.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "model not found locally and no remote server configured"})
}

// isLocalModel checks if a model is served by the local runtime.
func (h *lmHandler) isLocalModel(model string) bool {
	if model == "" || h.mgr == nil || h.localProxy == nil {
		return false
	}
	_, ok := h.localModels.Load(model)
	if ok {
		return true
	}
	// Also check without tag (e.g., "llama3.2" → "llama3.2:latest").
	if !strings.Contains(model, ":") {
		_, ok = h.localModels.Load(model + ":latest")
		return ok
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

	// Collect all known model IDs.
	var allModels []string
	h.localModels.Range(func(key, _ any) bool {
		allModels = append(allModels, key.(string))
		return true
	})
	for id := range h.cloudModels {
		allModels = append(allModels, id)
	}

	// Lazy-populate remote models if cache is empty and we have a remote proxy.
	// Uses singleflight to prevent thundering herd on concurrent first requests.
	remoteEmpty := true
	h.remoteModels.Range(func(_, _ any) bool {
		remoteEmpty = false
		return false // stop after first entry
	})
	if remoteEmpty && h.remoteProxy != nil {
		_, _, _ = h.remoteFetchGroup.Do("fetch-remote-models", func() (any, error) {
			req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
			if req != nil {
				remote := h.fetchRemoteModels(req)
				for _, m := range remote {
					h.remoteModels.Store(m.ID, true)
				}
			}
			return nil, nil
		})
	}

	h.remoteModels.Range(func(key, _ any) bool {
		allModels = append(allModels, key.(string))
		return true
	})

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
// Consistent with pkg/proxy/proxy.go's rewriteModelField: parse the body to
// find the actual current model string, then build a targeted regex that
// handles any whitespace around the colon and any valid JSON encoding of the
// old value (including \uXXXX escapes). Uses ReplaceAll with a ${1} backreference
// so the whitespace between the key and value is preserved.
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
	// Match "model"\s*:\s*<current-json-value>, preserving key+whitespace via capture group.
	// Use FindSubmatchIndex + manual splice so only the FIRST occurrence is replaced,
	// leaving any nested "model" keys in message content untouched.
	pattern := regexp.MustCompile(`("model"\s*:\s*)` + regexp.QuoteMeta(string(oldJSON)))
	loc := pattern.FindSubmatchIndex(body)
	if loc == nil {
		return body
	}
	// loc[0]:loc[1] = full match; loc[2]:loc[3] = capture group (key+whitespace).
	// Rebuild: everything before match + key+whitespace + newJSON + everything after match.
	result := make([]byte, 0, len(body)+(len(newJSON)-len(oldJSON)))
	result = append(result, body[:loc[2]]...)       // up to start of capture group
	result = append(result, body[loc[2]:loc[3]]...) // key + whitespace (the ${1} part)
	result = append(result, newJSON...)
	result = append(result, body[loc[1]:]...)
	return result
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
