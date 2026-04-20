package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/runtime"
)

// lmHandler implements a smart HTTP handler for the LM Studio compat listener.
// It intercepts /v1/models (merging local + remote models) and
// /v1/chat/completions (routing local models to the local runtime).
// All other paths pass through to the remote proxy.
type lmHandler struct {
	mgr          *runtime.Manager       // local runtime manager (may be nil)
	remoteProxy  *httputil.ReverseProxy // proxy to remote Candela server
	localProxy   *httputil.ReverseProxy // proxy to local runtime (e.g. Ollama)
	localHandler http.Handler           // localProxy wrapped with optional span capture

	localModels sync.Map // model ID string → bool (cached for fast routing)
}

// newLMHandler creates a smart LM compat handler that merges local + remote
// models and routes chat completions to the correct backend.
// If localHandler is non-nil, it wraps localProxy with span capture.
func newLMHandler(mgr *runtime.Manager, remoteProxy, localProxy *httputil.ReverseProxy, localHandler http.Handler) *lmHandler {
	if localHandler == nil && localProxy != nil {
		localHandler = localProxy
	}
	return &lmHandler{
		mgr:          mgr,
		remoteProxy:  remoteProxy,
		localProxy:   localProxy,
		localHandler: localHandler,
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

// serveModels merges local runtime models with remote cloud models.
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

	// 2. Fetch remote models by proxying to the remote Candela server.
	remoteModels := h.fetchRemoteModels(r)
	merged = append(merged, remoteModels...)

	// 3. Return merged OpenAI-format response.
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

// serveChat routes chat completions to the local runtime or remote server.
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

	// Replay body for the proxy.
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	if h.isLocalModel(req.Model) {
		slog.Debug("lm handler: routing to local runtime", "model", req.Model)
		h.localHandler.ServeHTTP(w, r)
	} else if h.remoteProxy != nil {
		h.remoteProxy.ServeHTTP(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model not found locally and no remote server configured"})
	}
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

// responseRecorder captures a proxy response for parsing.
type responseRecorder struct {
	headers    http.Header
	body       bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Header() http.Header { return r.headers }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK // match net/http implicit behavior
	}
	return r.body.Write(b)
}
func (r *responseRecorder) WriteHeader(code int) { r.statusCode = code }
