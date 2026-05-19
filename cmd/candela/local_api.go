package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/candelahq/candela/pkg/costcalc"
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
//	POST /_local/api/config/caching  — set caching mode at runtime (provider-agnostic)
type localAPI struct {
	mgr        *runtime.Manager
	cloudProxy *proxy.Proxy
	calc       *costcalc.Calculator // optional: for Gemini cache discount overrides
}

// registerLocalAPI mounts the /_local/* routes on the mux.
func registerLocalAPI(mux *http.ServeMux, mgr *runtime.Manager, cloudProxy *proxy.Proxy, calc *costcalc.Calculator) {
	api := &localAPI{mgr: mgr, cloudProxy: cloudProxy, calc: calc}

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

// cachingConfigResponse is the canonical config response for GET and POST.
// Having a shared type ensures the response shape is identical.
type cachingConfigResponse struct {
	Caching cachingProviders `json:"caching"`
}

type cachingProviders struct {
	Anthropic string        `json:"anthropic"`
	CacheTTL  string        `json:"cache_ttl"`
	Gemini    geminiCaching `json:"gemini"`
}

// geminiCaching surfaces Gemini's caching status. Gemini uses implicit caching
// (automatic, server-side) — the proxy doesn't inject anything. This section
// is informational for the desktop/CLI so users understand caching behavior.
type geminiCaching struct {
	Mode          string `json:"mode"`           // always "implicit" — automatic server-side caching
	CacheDiscount string `json:"cache_discount"` // e.g. "90%" (2.5+) or "75%" (2.0)
	Info          string `json:"info"`           // human-readable explanation
}

// buildCachingConfig constructs the canonical caching config state.
func (a *localAPI) buildCachingConfig() cachingProviders {
	cachingMode := string(proxy.CachingOff)
	cacheTTL := string(proxy.CacheTTL5m)
	if a.cloudProxy != nil {
		cachingMode = string(a.cloudProxy.GetCachingMode())
		cacheTTL = string(a.cloudProxy.GetCacheTTL())
	}

	discountStr := "90% (2.5+/3.x), 75% (2.0)"
	if a.calc != nil {
		if dc, ok := a.calc.GetCacheDiscount("google"); ok {
			discountStr = fmt.Sprintf("%.0f%% (runtime override)", dc.ReadDiscount*100)
		}
	}

	return cachingProviders{
		Anthropic: cachingMode,
		CacheTTL:  cacheTTL,
		Gemini: geminiCaching{
			Mode:          "implicit",
			CacheDiscount: discountStr,
			Info:          "Gemini caching is automatic. Repeated prefixes are cached server-side with no client-side injection needed.",
		},
	}
}

// GET /_local/api/config — returns current runtime configuration state.
func (a *localAPI) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, cachingConfigResponse{
		Caching: a.buildCachingConfig(),
	})
}

// POST /_local/api/config/caching — set caching mode and TTL at runtime.
//
// Body: {"anthropic": "auto", "cache_ttl": "1h", "gemini_cache_discount": 0.10}
//   - anthropic: off, auto, system-only
//   - cache_ttl: 5m, 1h (Anthropic only — Gemini TTL is managed server-side)
//   - gemini_cache_discount: 0.0–1.0 override for cache read discount (optional)
//
// All fields are optional; omitted fields are left unchanged.
func (a *localAPI) handleSetCaching(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		Anthropic           *string  `json:"anthropic"`
		CacheTTL            *string  `json:"cache_ttl"`
		GeminiCacheDiscount *float64 `json:"gemini_cache_discount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if a.cloudProxy == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "proxy not initialized",
		})
		return
	}

	if req.Anthropic != nil {
		mode := proxy.ParseCachingMode(*req.Anthropic)
		a.cloudProxy.SetCachingMode(mode)
		slog.Info("caching mode updated at runtime", "anthropic", string(mode))
	}

	if req.CacheTTL != nil {
		ttl := proxy.ParseCacheTTL(*req.CacheTTL)
		a.cloudProxy.SetCacheTTL(ttl)
		slog.Info("cache TTL updated at runtime", "cache_ttl", string(ttl))
	}

	// Gemini cache discount override — allows enterprise customers to set
	// custom rates if they have negotiated pricing.
	if req.GeminiCacheDiscount != nil && a.calc != nil {
		disc := *req.GeminiCacheDiscount
		if disc < 0 || disc > 1.0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gemini_cache_discount must be between 0.0 and 1.0"})
			return
		}
		a.calc.SetCacheDiscount("google", costcalc.CacheDiscountConfig{
			ReadDiscount:       disc,
			CreateMultiplier:   1.0,
			InputIncludesCache: true,
		})
		slog.Info("Gemini cache discount updated at runtime", "discount", disc)
	}

	writeJSON(w, http.StatusOK, cachingConfigResponse{
		Caching: a.buildCachingConfig(),
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
