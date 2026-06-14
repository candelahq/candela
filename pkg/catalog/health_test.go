package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

func TestCheckHealth_ConfigStore(t *testing.T) {
	store := catalog.NewConfigStore(nil)
	status := catalog.CheckHealth(context.Background(), store)

	if !status.Healthy {
		t.Errorf("expected healthy, got error: %s", status.Error)
	}
	if status.Backend != "config" {
		t.Errorf("backend = %q, want %q", status.Backend, "config")
	}
	if status.Writable {
		t.Error("ConfigStore should not be writable")
	}
	if status.LatencyMs < 0 {
		t.Error("expected non-negative latency")
	}
}

func TestCheckHealth_NilStore(t *testing.T) {
	status := catalog.CheckHealth(context.Background(), nil)
	if status.Healthy {
		t.Error("nil store should not be healthy")
	}
	if status.Error == "" {
		t.Error("nil store should have an error message")
	}
}

func TestCheckHealth_LatencyIsMilliseconds(t *testing.T) {
	store := catalog.NewConfigStore(nil)
	status := catalog.CheckHealth(context.Background(), store)
	// Latency should be a reasonable millisecond value, not nanoseconds
	if status.LatencyMs > 1000 {
		t.Errorf("latency_ms = %f, suspiciously high (nanoseconds leaked?)", status.LatencyMs)
	}
	if status.LatencyMs < 0 {
		t.Errorf("latency_ms = %f, should be non-negative", status.LatencyMs)
	}
}

func TestCheckHealth_CancelledContext(t *testing.T) {
	store := catalog.NewConfigStore(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status := catalog.CheckHealth(ctx, store)
	// Just verify it doesn't panic and returns in bounded time
	if status.LatencyMs < 0 {
		t.Error("latency should be non-negative even on error")
	}
}

func TestHealthStatus_JSONSerialization(t *testing.T) {
	status := catalog.HealthStatus{
		Healthy:    true,
		Backend:    "config",
		ModelCount: 42,
		LatencyMs:  1.5,
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["latency_ms"] != 1.5 {
		t.Errorf("latency_ms = %v, want 1.5", decoded["latency_ms"])
	}
}
