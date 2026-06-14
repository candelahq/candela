package catalog

import (
	"context"
	"time"
)

// HealthStatus represents the health of the catalog store.
type HealthStatus struct {
	Healthy    bool    `json:"healthy"`
	Backend    string  `json:"backend"`
	Writable   bool    `json:"writable"`
	ModelCount int     `json:"model_count"`
	LatencyMs  float64 `json:"latency_ms"`
	Error      string  `json:"error,omitempty"`
}

// CheckHealth performs a health check on the catalog store.
func CheckHealth(ctx context.Context, store ModelCatalogStore) HealthStatus {
	if store == nil {
		return HealthStatus{Error: "catalog store is nil"}
	}

	start := time.Now()
	status := HealthStatus{
		Backend:  store.Source(),
		Writable: store.Writable(),
	}

	entries, err := store.List(ctx, false)
	status.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		status.Error = "catalog list failed"
		return status
	}

	status.Healthy = true
	status.ModelCount = len(entries)
	return status
}
