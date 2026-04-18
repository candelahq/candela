package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/candelahq/candela/pkg/runtime"
)

// localAPI provides the /_local/* management endpoints for controlling
// the local LLM runtime from the embedded UI or curl.
//
// Endpoints:
//
//	GET  /_local/health        — runtime health + loaded models
//	GET  /_local/models        — list available models
//	POST /_local/models/pull   — pull/download a model
//	GET  /_local/backends      — list registered runtime backends
type localAPI struct {
	mgr *runtime.Manager
}

// registerLocalAPI mounts the /_local/* routes on the mux.
func registerLocalAPI(mux *http.ServeMux, mgr *runtime.Manager) {
	api := &localAPI{mgr: mgr}

	mux.HandleFunc("GET /_local/health", api.handleHealth)
	mux.HandleFunc("GET /_local/models", api.handleListModels)
	mux.HandleFunc("POST /_local/models/pull", api.handlePullModel)
	mux.HandleFunc("GET /_local/backends", api.handleBackends)
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
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": err.Error(),
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
			"error": "invalid JSON: " + err.Error(),
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

// writeJSON is a helper for consistent JSON responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
