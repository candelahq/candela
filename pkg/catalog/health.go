package catalog

import (
	"context"
	"fmt"
	"time"
)

// HealthStatus represents the health of the catalog store.
type HealthStatus struct {
	Healthy    bool          `json:"healthy"`
	Backend    string        `json:"backend"`
	Writable   bool          `json:"writable"`
	ModelCount int           `json:"model_count"`
	Latency    time.Duration `json:"latency_ms"`
	Error      string        `json:"error,omitempty"`
}

// CheckHealth performs a health check on the catalog store.
func CheckHealth(ctx context.Context, store ModelCatalogStore) HealthStatus {
	start := time.Now()
	status := HealthStatus{
		Backend:  store.Source(),
		Writable: store.Writable(),
	}

	entries, err := store.List(ctx, false)
	status.Latency = time.Since(start)

	if err != nil {
		status.Error = fmt.Sprintf("list failed: %v", err)
		return status
	}

	status.Healthy = true
	status.ModelCount = len(entries)
	return status
}
