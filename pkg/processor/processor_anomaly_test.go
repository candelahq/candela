package processor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/anomaly"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// mockAnomalyReader implements storage.SpanReader for the anomaly detector.
// It returns configurable spans from SearchSpans and ignores all other methods.
type mockAnomalyReader struct {
	spans     []storage.Span
	searchErr error
}

func (m *mockAnomalyReader) SearchSpans(_ context.Context, _ storage.SpanQuery) (*storage.SpanResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return &storage.SpanResult{Spans: m.spans, TotalCount: len(m.spans)}, nil
}

func (m *mockAnomalyReader) GetTrace(_ context.Context, _ string) (*storage.Trace, error) {
	return nil, nil
}
func (m *mockAnomalyReader) QueryTraces(_ context.Context, _ storage.TraceQuery) (*storage.TraceResult, error) {
	return nil, nil
}
func (m *mockAnomalyReader) GetUsageSummary(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, error) {
	return nil, nil
}
func (m *mockAnomalyReader) GetModelBreakdown(_ context.Context, _ storage.UsageQuery) ([]storage.ModelUsage, error) {
	return nil, nil
}
func (m *mockAnomalyReader) GetUserLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.UserUsageSummary, error) {
	return nil, nil
}
func (m *mockAnomalyReader) GetTenantLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.TenantUsageSummary, error) {
	return nil, nil
}
func (m *mockAnomalyReader) GetJobLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.JobUsageSummary, error) {
	return nil, nil
}
func (m *mockAnomalyReader) Ping(_ context.Context) error { return nil }
func (m *mockAnomalyReader) Close() error                 { return nil }

// anomalyTestSpan builds an LLM span with specific cost and duration for anomaly testing.
func anomalyTestSpan(id string, costUSD float64, durationMs int64) storage.Span {
	now := time.Now().UTC()
	return storage.Span{
		SpanID:    id,
		TraceID:   "trace-" + id,
		Name:      "test." + id,
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(time.Duration(durationMs) * time.Millisecond),
		Duration:  time.Duration(durationMs) * time.Millisecond,
		ProjectID: "proj-test",
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			CostUSD:      costUSD,
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}
}

func TestProcessor_AnomalyAttributePropagation(t *testing.T) {
	// When the anomaly detector flags a span, the processor should propagate
	// candela.anomaly and candela.anomaly_reason attributes onto the batch
	// span that gets written to sinks.
	w := &mockWriter{}
	calc := costcalc.New()

	// Build a varied historical baseline (alternating costs for non-zero stddev).
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = anomalyTestSpan(fmt.Sprintf("h%d", i), cost, 100)
	}

	reader := &mockAnomalyReader{spans: history}
	detector := anomaly.New(reader, anomaly.DefaultConfig())

	proc := New([]storage.SpanWriter{w}, calc, 1) // batch size 1 → flush immediately
	proc.WithAnomalyDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Submit a cost spike that should be flagged.
	spike := anomalyTestSpan("spike-1", 5.00, 100)
	proc.Submit(spike)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span written to sink")
	}

	written := spans[0]
	if written.Attributes == nil {
		t.Fatal("expected Attributes map to be non-nil on written span")
	}
	if written.Attributes[anomaly.AttrAnomaly] != "true" {
		t.Errorf("expected %s = 'true', got %q", anomaly.AttrAnomaly, written.Attributes[anomaly.AttrAnomaly])
	}
	if written.Attributes[anomaly.AttrAnomalyReason] == "" {
		t.Error("expected non-empty anomaly reason attribute on flagged span")
	}
}

func TestProcessor_AnomalyDetectorError_DoesNotBlockWrite(t *testing.T) {
	// When the anomaly detector returns an error (e.g. storage unavailable),
	// the processor should still write spans to sinks (graceful degradation).
	w := &mockWriter{}
	calc := costcalc.New()

	reader := &mockAnomalyReader{searchErr: fmt.Errorf("storage timeout")}
	detector := anomaly.New(reader, anomaly.DefaultConfig())

	proc := New([]storage.SpanWriter{w}, calc, 1)
	proc.WithAnomalyDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	span := anomalyTestSpan("s1", 0.01, 100)
	proc.Submit(span)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected spans to be written even when anomaly detector errors")
	}
	// Span should NOT have anomaly attributes when detection failed.
	if spans[0].Attributes != nil && spans[0].Attributes[anomaly.AttrAnomaly] == "true" {
		t.Error("span should not be flagged when anomaly detection errors")
	}
}

func TestProcessor_NilDetector_NoOp(t *testing.T) {
	// When no detector is attached, the processor should function normally
	// without panics or anomaly attributes.
	w := &mockWriter{}
	calc := costcalc.New()

	proc := New([]storage.SpanWriter{w}, calc, 1)
	// Deliberately not calling WithAnomalyDetector.

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	span := anomalyTestSpan("no-detector", 0.01, 100)
	proc.Submit(span)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span written to sink")
	}
	// No anomaly attributes should be present.
	if spans[0].Attributes != nil && spans[0].Attributes[anomaly.AttrAnomaly] != "" {
		t.Error("anomaly attribute should not be set when no detector is attached")
	}
}

func TestProcessor_AnomalyNilAttributes_Initialized(t *testing.T) {
	// Verify that if a span has nil Attributes before anomaly detection, the
	// processor initializes the map and adds anomaly attributes correctly.
	w := &mockWriter{}
	calc := costcalc.New()

	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = anomalyTestSpan(fmt.Sprintf("h%d", i), cost, 100)
	}

	reader := &mockAnomalyReader{spans: history}
	detector := anomaly.New(reader, anomaly.DefaultConfig())

	proc := New([]storage.SpanWriter{w}, calc, 1)
	proc.WithAnomalyDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Submit a spike with explicitly nil Attributes.
	spike := anomalyTestSpan("nil-attrs", 5.00, 100)
	spike.Attributes = nil
	proc.Submit(spike)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span written to sink")
	}
	if spans[0].Attributes == nil {
		t.Fatal("expected Attributes map to be initialized for flagged span with nil Attributes")
	}
	if spans[0].Attributes[anomaly.AttrAnomaly] != "true" {
		t.Errorf("expected %s = 'true' on span with initially nil Attributes", anomaly.AttrAnomaly)
	}
}

func TestProcessor_AnomalyPreservesExistingAttributes(t *testing.T) {
	// When a span already has Attributes, anomaly detection should ADD
	// the anomaly keys without clobbering existing ones.
	w := &mockWriter{}
	calc := costcalc.New()

	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = anomalyTestSpan(fmt.Sprintf("h%d", i), cost, 100)
	}

	reader := &mockAnomalyReader{spans: history}
	detector := anomaly.New(reader, anomaly.DefaultConfig())

	proc := New([]storage.SpanWriter{w}, calc, 1)
	proc.WithAnomalyDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	spike := anomalyTestSpan("existing-attrs", 5.00, 100)
	spike.Attributes = map[string]string{
		"user.custom_tag": "important",
		"env":             "production",
	}
	proc.Submit(spike)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span written to sink")
	}

	attrs := spans[0].Attributes
	// Check anomaly attributes were added.
	if attrs[anomaly.AttrAnomaly] != "true" {
		t.Error("anomaly attribute not set on flagged span")
	}
	// Check existing attributes were preserved.
	if attrs["user.custom_tag"] != "important" {
		t.Errorf("existing attribute user.custom_tag = %q, want 'important'", attrs["user.custom_tag"])
	}
	if attrs["env"] != "production" {
		t.Errorf("existing attribute env = %q, want 'production'", attrs["env"])
	}
}

func TestProcessor_AnomalyNormalSpan_NoFlag(t *testing.T) {
	// A span within normal range should NOT have anomaly attributes after
	// passing through the processor with an active detector.
	w := &mockWriter{}
	calc := costcalc.New()

	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = anomalyTestSpan(fmt.Sprintf("h%d", i), cost, 100)
	}

	reader := &mockAnomalyReader{spans: history}
	detector := anomaly.New(reader, anomaly.DefaultConfig())

	proc := New([]storage.SpanWriter{w}, calc, 1)
	proc.WithAnomalyDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Submit a normal span (cost within baseline range).
	normal := anomalyTestSpan("normal-1", 0.01, 100)
	proc.Submit(normal)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span written to sink")
	}
	if spans[0].Attributes != nil && spans[0].Attributes[anomaly.AttrAnomaly] == "true" {
		t.Error("normal span should not be flagged as anomalous")
	}
}
