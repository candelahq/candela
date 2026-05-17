package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/runtime"
)

// localAPI provides the /_local/* management endpoints for controlling
// the local LLM runtime from the embedded UI or curl.
//
// Endpoints:
//
//	GET  /_local/health              — runtime health + loaded models
//	GET  /_local/models              — list available models
//	POST /_local/models/pull         — pull/download a model
//	GET  /_local/backends            — list registered runtime backends
//	GET  /_local/api/config          — current runtime configuration
//	POST /_local/api/config/caching  — set Anthropic caching mode at runtime
type localAPI struct {
	mgr        *runtime.Manager
	cloudProxy *proxy.Proxy
}

// registerLocalAPI mounts the /_local/* routes on the mux.
func registerLocalAPI(mux *http.ServeMux, mgr *runtime.Manager, cloudProxy *proxy.Proxy) {
	api := &localAPI{mgr: mgr, cloudProxy: cloudProxy}

	if mgr != nil {
		mux.HandleFunc("GET /_local/health", api.handleHealth)
		mux.HandleFunc("GET /_local/models", api.handleListModels)
		mux.HandleFunc("POST /_local/models/pull", api.handlePullModel)
		mux.HandleFunc("GET /_local/backends", api.handleBackends)
	}
	mux.HandleFunc("GET /_local/api/config", api.handleGetConfig)
	mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)
}

// GET /_local/health
func (a *localAPI) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h := a.mgr.Health()
	writeJSON(w, http.StatusOK, h)
}

// GET /_local/models
func (a *localAPI) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := a.mgr.Runtime().ListModels(r.Context())
	if err != nil {
		slog.Warn("failed to list local models", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "failed to list local models",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models": models,
	})
}

// POST /_local/models/pull
// Body: {"model": "llama3.2:8b"}
func (a *localAPI) handlePullModel(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // Limit body to 1MB
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "model is required",
		})
		return
	}

	slog.Info("pulling model via API", "model", req.Model)

	// Pull in a goroutine and return immediately with 202 Accepted.
	// The UI can poll /_local/health to see when the model appears.
	go func() {
		// Use context.Background() because r.Context() is cancelled when the handler returns.
		ctx := context.Background()
		progress := make(chan runtime.PullProgress, 16)
		defer close(progress)
		go func() {
			for p := range progress {
				slog.Info("pull progress",
					"model", req.Model,
					"status", p.Status,
					"percent", p.Percent,
				)
			}
		}()
		if err := a.mgr.Runtime().PullModel(ctx, req.Model, progress); err != nil {
			slog.Error("model pull failed", "model", req.Model, "error", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "pulling",
		"model":  req.Model,
	})
}

// GET /_local/backends
func (a *localAPI) handleBackends(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"backends": runtime.Names(),
	})
}

// GET /_local/api/config — returns current runtime configuration state.
func (a *localAPI) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	cachingMode := string(proxy.CachingOff)
	if a.cloudProxy != nil {
		cachingMode = string(a.cloudProxy.GetCachingMode())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"caching": map[string]string{
			"anthropic": cachingMode,
		},
	})
}

// POST /_local/api/config/caching — set caching mode at runtime.
// Body: {"anthropic": "auto"} — values: off, auto, system-only
func (a *localAPI) handleSetCaching(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		Anthropic string `json:"anthropic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	mode := proxy.ParseCachingMode(req.Anthropic)
	if a.cloudProxy == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "proxy not initialized",
		})
		return
	}
	a.cloudProxy.SetCachingMode(mode)
	slog.Info("caching mode updated at runtime", "anthropic", string(mode))
	writeJSON(w, http.StatusOK, map[string]any{
		"caching": map[string]string{
			"anthropic": string(mode),
		},
	})
}

// writeJSON is a helper for consistent JSON responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
