package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// tracesHandler serves the /_local/api/traces REST endpoint for querying
// locally-captured spans in solo mode.
type tracesHandler struct {
	reader storage.SpanReader
}

// recentSpanResponse is the JSON response for the traces endpoint.
type recentSpanResponse struct {
	Spans []spanJSON `json:"spans"`
	Count int        `json:"count"`
}

type spanJSON struct {
	SpanID       string  `json:"span_id"`
	TraceID      string  `json:"trace_id"`
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMs   float64 `json:"duration_ms"`
	Status       string  `json:"status"`
	Timestamp    string  `json:"timestamp"`
	Name         string  `json:"name"`
}

func newTracesHandler(reader storage.SpanReader) http.Handler {
	if reader == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "traces not available — no storage configured"})
		})
	}
	return &tracesHandler{reader: reader}
}

func (h *tracesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit from query param (default 500, max 2000).
	// The Flutter client requests 500 for 24h and 2000 for 7d/30d views;
	// a 200-span cap was silently truncating billing data.
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 2000 {
		limit = 2000
	}

	// Parse range query param to set the correct time window.
	// The Flutter client sends "24h", "7d", or "30d".
	// Without this, the server always returned 7-day data regardless
	// of the selected range, making the 30d chart show only 7d totals.
	now := time.Now()
	window := 7 * 24 * time.Hour // default: 7 days
	switch r.URL.Query().Get("range") {
	case "24h":
		window = 24 * time.Hour
	case "30d":
		window = 30 * 24 * time.Hour
	}
	startTime := now.Add(-window)

	result, err := h.reader.SearchSpans(r.Context(), storage.SpanQuery{
		ProjectID: "local",
		StartTime: startTime,
		EndTime:   now,
		Kind:      storage.SpanKindLLM,
		PageSize:  limit,
	})
	if err != nil {
		slog.Error("failed to query traces", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to query traces"})
		return
	}

	// Convert to JSON response.
	spans := make([]spanJSON, 0, len(result.Spans))
	for _, s := range result.Spans {
		sj := spanJSON{
			SpanID:     s.SpanID,
			TraceID:    s.TraceID,
			Name:       s.Name,
			DurationMs: float64(s.Duration.Milliseconds()),
			Status:     statusString(s.Status),
			Timestamp:  s.StartTime.Format(time.RFC3339),
		}
		if s.GenAI != nil {
			sj.Model = s.GenAI.Model
			sj.Provider = s.GenAI.Provider
			sj.InputTokens = s.GenAI.InputTokens
			sj.OutputTokens = s.GenAI.OutputTokens
			sj.TotalTokens = s.GenAI.TotalTokens
			sj.CostUSD = s.GenAI.CostUSD
		}
		spans = append(spans, sj)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recentSpanResponse{
		Spans: spans,
		Count: len(spans),
	})
}

func statusString(s storage.SpanStatus) string {
	switch s {
	case storage.SpanStatusOK:
		return "ok"
	case storage.SpanStatusError:
		return "error"
	default:
		return "unset"
	}
}
