package otel_test

import (
	"context"
	"testing"

	promclient "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	candelaotel "github.com/candelahq/candela/pkg/otel"
)

// newIsolatedRegistry creates both a MeterProvider and its backing
// Prometheus registry so tests can record values and then gather/inspect
// the resulting metric families.
func newIsolatedRegistry(t *testing.T) (*sdkmetric.MeterProvider, *promclient.Registry) {
	t.Helper()
	reg := promclient.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		t.Fatalf("creating prometheus exporter: %v", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return mp, reg
}

// gatherMetric is a helper that gathers all metric families from the
// registry and returns the one matching the given name, or nil.
func gatherMetric(t *testing.T, reg *promclient.Registry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1. All instruments initialised
// ---------------------------------------------------------------------------

func TestProxyMetrics_AllInstrumentsInitialized(t *testing.T) {
	mp, _ := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatalf("NewProxyMetrics: %v", err)
	}

	checks := []struct {
		name string
		ok   bool
	}{
		{"RequestDuration", m.RequestDuration != nil},
		{"RequestTotal", m.RequestTotal != nil},
		{"ActiveRequests", m.ActiveRequests != nil},
		{"TokensProcessed", m.TokensProcessed != nil},
		{"CostUSD", m.CostUSD != nil},
		{"DroppedSpans", m.DroppedSpans != nil},
		{"DroppedAsync", m.DroppedAsync != nil},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("field %s is nil", c.name)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Nil MeterProvider
// ---------------------------------------------------------------------------

func TestProxyMetrics_NilProvider(t *testing.T) {
	// Passing a nil MeterProvider should panic (it dereferences mp.Meter()).
	// Verify we don't silently succeed with nils.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil MeterProvider, but got none")
		}
	}()
	_, _ = candelaotel.NewProxyMetrics(nil)
}

// ---------------------------------------------------------------------------
// 3. RequestDuration – record and read back
// ---------------------------------------------------------------------------

func TestProxyMetrics_RequestDuration_Record(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.RequestDuration.Record(ctx, 0.250)
	m.RequestDuration.Record(ctx, 0.750)

	// The Prometheus exporter translates OTel metric names by replacing
	// dots with underscores and may add a _seconds suffix for histograms.
	// OTel histogram names appear as-is but with dots → underscores.
	fam := gatherMetric(t, reg, "candela_proxy_request_duration_seconds")
	if fam == nil {
		// Fallback: try without the unit suffix (depends on exporter version).
		fam = gatherMetric(t, reg, "candela_proxy_request_duration")
	}
	if fam == nil {
		// Dump all metric names to help debug.
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("histogram metric not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points in histogram")
	}
	h := metrics[0].GetHistogram()
	if h == nil {
		t.Fatal("expected histogram data")
	}
	if got := h.GetSampleCount(); got != 2 {
		t.Errorf("sample count = %d, want 2", got)
	}
	if got := h.GetSampleSum(); got != 1.0 {
		t.Errorf("sample sum = %f, want 1.0", got)
	}
}

// ---------------------------------------------------------------------------
// 4. RequestTotal – increment with labels
// ---------------------------------------------------------------------------

func TestProxyMetrics_RequestTotal_Increment(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	// Record several requests — all aggregate under the same label set.
	m.RequestTotal.Add(ctx, 1)
	m.RequestTotal.Add(ctx, 1)
	m.RequestTotal.Add(ctx, 3)

	fam := gatherMetric(t, reg, "candela_proxy_request_total")
	if fam == nil {
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("counter metric not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points")
	}
	// All adds were without distinguishing attributes, so they aggregate.
	got := metrics[0].GetCounter().GetValue()
	if got != 5 {
		t.Errorf("counter value = %f, want 5", got)
	}
}

// ---------------------------------------------------------------------------
// 5. ActiveRequests – up/down gauge
// ---------------------------------------------------------------------------

func TestProxyMetrics_ActiveRequests_UpDown(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.ActiveRequests.Add(ctx, 3)  // +3
	m.ActiveRequests.Add(ctx, 2)  // +2 → 5
	m.ActiveRequests.Add(ctx, -4) // -4 → 1

	fam := gatherMetric(t, reg, "candela_proxy_request_active")
	if fam == nil {
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("gauge metric not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points")
	}
	got := metrics[0].GetGauge().GetValue()
	if got != 1 {
		t.Errorf("gauge value = %f, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// 6. TokensProcessed – record with labels
// ---------------------------------------------------------------------------

func TestProxyMetrics_TokensProcessed_Record(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.TokensProcessed.Add(ctx, 100)
	m.TokensProcessed.Add(ctx, 250)

	fam := gatherMetric(t, reg, "candela_proxy_tokens_processed_total")
	if fam == nil {
		fam = gatherMetric(t, reg, "candela_proxy_tokens_processed")
	}
	if fam == nil {
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("tokens counter not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points")
	}
	got := metrics[0].GetCounter().GetValue()
	if got != 350 {
		t.Errorf("tokens counter = %f, want 350", got)
	}
}

// ---------------------------------------------------------------------------
// 7. CostUSD – record cost values
// ---------------------------------------------------------------------------

func TestProxyMetrics_CostUSD_Record(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.CostUSD.Add(ctx, 0.0025)
	m.CostUSD.Add(ctx, 0.0075)

	fam := gatherMetric(t, reg, "candela_proxy_cost_usd_USD_total")
	if fam == nil {
		fam = gatherMetric(t, reg, "candela_proxy_cost_usd_total")
	}
	if fam == nil {
		fam = gatherMetric(t, reg, "candela_proxy_cost_usd")
	}
	if fam == nil {
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("cost counter not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points")
	}
	got := metrics[0].GetCounter().GetValue()
	want := 0.01
	if got < want-0.0001 || got > want+0.0001 {
		t.Errorf("cost counter = %f, want %f", got, want)
	}
}

// ---------------------------------------------------------------------------
// 8. DroppedSpans – verify counter works
// ---------------------------------------------------------------------------

func TestProxyMetrics_DroppedSpans_Increment(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.DroppedSpans.Add(ctx, 7)

	fam := gatherMetric(t, reg, "candela_proxy_spans_dropped_total")
	if fam == nil {
		fam = gatherMetric(t, reg, "candela_proxy_spans_dropped")
	}
	if fam == nil {
		families, _ := reg.Gather()
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Fatalf("dropped spans counter not found; available: %v", names)
	}

	metrics := fam.GetMetric()
	if len(metrics) == 0 {
		t.Fatal("no data points")
	}
	got := metrics[0].GetCounter().GetValue()
	if got != 7 {
		t.Errorf("dropped spans = %f, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// 9. MetricNames – verify candela.proxy.* namespace
// ---------------------------------------------------------------------------

func TestProxyMetrics_MetricNames(t *testing.T) {
	mp, reg := newIsolatedRegistry(t)
	m, err := candelaotel.NewProxyMetrics(mp)
	if err != nil {
		t.Fatal(err)
	}

	// Record at least one value on every instrument so they appear in the
	// Prometheus registry (lazy initialisation).
	ctx := context.Background()
	m.RequestDuration.Record(ctx, 1.0)
	m.RequestTotal.Add(ctx, 1)
	m.ActiveRequests.Add(ctx, 1)
	m.TokensProcessed.Add(ctx, 1)
	m.CostUSD.Add(ctx, 1)
	m.DroppedSpans.Add(ctx, 1)
	m.DroppedAsync.Add(ctx, 1)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	// Every metric name should start with "candela_proxy_" (the Prometheus
	// exporter converts dots to underscores).
	const prefix = "candela_proxy_"
	found := 0
	for _, f := range families {
		name := f.GetName()
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			found++
		}
	}

	// We expect at least 7 distinct metric families (one per instrument).
	if found < 7 {
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Errorf("expected ≥ 7 candela_proxy_* metrics, got %d; names: %v", found, names)
	}
}
