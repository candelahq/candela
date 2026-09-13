package otel_test

import (
	"context"
	"sync"
	"testing"

	promclient "github.com/prometheus/client_golang/prometheus"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	candelaotel "github.com/candelahq/candela/pkg/otel"
)

// newIsolatedMeterProvider creates a MeterProvider backed by a fresh
// Prometheus registry so each test is hermetic and never collides with
// the global default registry.
func newIsolatedMeterProvider(t *testing.T) *sdkmetric.MeterProvider {
	t.Helper()
	reg := promclient.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		t.Fatalf("creating prometheus exporter: %v", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return mp
}

func TestSetup(t *testing.T) {
	reg := promclient.NewRegistry()
	shutdown, err := candelaotel.Setup(context.Background(), candelaotel.Config{
		ServiceName:    "candela-test",
		ServiceVersion: "0.0.0-test",
		Registry:       reg,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

func TestSetup_DefaultRegistry(t *testing.T) {
	// Ensure Setup still works when Registry is nil (uses default).
	shutdown, err := candelaotel.Setup(context.Background(), candelaotel.Config{
		ServiceName:    "candela-default-reg",
		ServiceVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("Setup with nil registry failed: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

type captureExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *captureExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *captureExporter) Shutdown(context.Context) error {
	return nil
}

func TestSetup_WithSpanExporter(t *testing.T) {
	spanExporter := &captureExporter{}
	reg := promclient.NewRegistry()
	shutdown, err := candelaotel.Setup(context.Background(), candelaotel.Config{
		ServiceName:    "candela-trace-test",
		ServiceVersion: "0.0.0-test",
		Registry:       reg,
		SpanExporter:   spanExporter,
	})
	if err != nil {
		t.Fatalf("Setup with SpanExporter failed: %v", err)
	}

	tracer := candelaotel.Tracer("setup-test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	// Shutdown flushes the batcher
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	spanExporter.mu.Lock()
	defer spanExporter.mu.Unlock()
	if len(spanExporter.spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spanExporter.spans))
	}
	if spanExporter.spans[0].Name() != "test-span" {
		t.Errorf("expected span name 'test-span', got %q", spanExporter.spans[0].Name())
	}
}

func TestNewProxyMetrics(t *testing.T) {
	mp := newIsolatedMeterProvider(t)

	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}
	if m.RequestTotal == nil {
		t.Error("RequestTotal is nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}
	if m.ActiveRequests == nil {
		t.Error("ActiveRequests is nil")
	}
	if m.TokensProcessed == nil {
		t.Error("TokensProcessed is nil")
	}
	if m.CostUSD == nil {
		t.Error("CostUSD is nil")
	}
	if m.DroppedSpans == nil {
		t.Error("DroppedSpans is nil")
	}
	if m.DroppedAsync == nil {
		t.Error("DroppedAsync is nil")
	}
}

// TestNewProxyMetrics_Parallel proves that two isolated providers
// can be created concurrently without registration collisions.
func TestNewProxyMetrics_Parallel(t *testing.T) {
	for _, name := range []string{"a", "b", "c"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mp := newIsolatedMeterProvider(t)
			m, err := candelaotel.NewProxyMetrics(mp)
			if err != nil {
				t.Fatal(err)
			}
			if m.RequestTotal == nil {
				t.Error("RequestTotal is nil")
			}
		})
	}
}

func TestMeter(t *testing.T) {
	// Meter is a thin wrapper over the global provider; just verify it
	// returns a non-nil value without panicking.
	m := candelaotel.Meter("test.meter")
	if m == nil {
		t.Error("Meter returned nil")
	}
}
