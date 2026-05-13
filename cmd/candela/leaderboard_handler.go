package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// leaderboardHandler serves the /_local/api/leaderboard REST endpoint.
type leaderboardHandler struct {
	reader storage.SpanReader
}

func newLeaderboardHandler(reader storage.SpanReader) http.Handler {
	if reader == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "leaderboard not available — no storage configured"})
		})
	}
	return &leaderboardHandler{reader: reader}
}

func (h *leaderboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	now := time.Now()
	monthAgo := now.Add(-30 * 24 * time.Hour)

	tenants, err := h.reader.GetTenantLeaderboard(r.Context(), storage.UsageQuery{
		ProjectID: "local",
		StartTime: monthAgo,
		EndTime:   now,
	}, limit)
	if err != nil {
		slog.Error("failed to query tenant leaderboard", "error", err)
		// Don't fail the whole request if one leaderboard fails.
	}

	jobs, err := h.reader.GetJobLeaderboard(r.Context(), storage.UsageQuery{
		ProjectID: "local",
		StartTime: monthAgo,
		EndTime:   now,
	}, limit)
	if err != nil {
		slog.Error("failed to query job leaderboard", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenants": tenants,
		"jobs":    jobs,
		"count":   len(tenants) + len(jobs),
	})
}
