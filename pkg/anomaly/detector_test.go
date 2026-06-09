package anomaly

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// mockReader is a SpanReader stub that returns a fixed set of spans for SearchSpans.
type mockReader struct {
	spans []storage.Span
}

func (m *mockReader) SearchSpans(_ context.Context, _ storage.SpanQuery) (*storage.SpanResult, error) {
	return &storage.SpanResult{Spans: m.spans, TotalCount: len(m.spans)}, nil
}

func (m *mockReader) GetTrace(_ context.Context, _ string) (*storage.Trace, error) { return nil, nil }
func (m *mockReader) QueryTraces(_ context.Context, _ storage.TraceQuery) (*storage.TraceResult, error) {
	return nil, nil
}
func (m *mockReader) GetUsageSummary(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, error) {
	return nil, nil
}
func (m *mockReader) GetModelBreakdown(_ context.Context, _ storage.UsageQuery) ([]storage.ModelUsage, error) {
	return nil, nil
}
func (m *mockReader) GetUserLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.UserUsageSummary, error) {
	return nil, nil
}
func (m *mockReader) GetTenantLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.TenantUsageSummary, error) {
	return nil, nil
}
func (m *mockReader) GetJobLeaderboard(_ context.Context, _ storage.UsageQuery, _ int) ([]storage.JobUsageSummary, error) {
	return nil, nil
}
func (m *mockReader) Ping(_ context.Context) error { return nil }
func (m *mockReader) Close() error                 { return nil }

// llmSpan builds a minimal LLM span with the given cost and duration.
func llmSpan(costUSD float64, durationMs int64) storage.Span {
	now := time.Now().UTC()
	return storage.Span{
		SpanID:    "test-span",
		ProjectID: "proj",
		Kind:      storage.SpanKindLLM,
		StartTime: now,
		EndTime:   now.Add(time.Duration(durationMs) * time.Millisecond),
		Duration:  time.Duration(durationMs) * time.Millisecond,
		GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", CostUSD: costUSD},
	}
}

// baseline builds N identical spans with the given cost and latency to use as history.
func baseline(n int, costUSD float64, durationMs int64) []storage.Span {
	spans := make([]storage.Span, n)
	for i := range spans {
		spans[i] = llmSpan(costUSD, durationMs)
	}
	return spans
}

func TestDetect_CostAnomaly(t *testing.T) {
	// varied baseline: alternating $0.009/$0.011, mean=$0.01, stddev=$0.001.
	// spike at $1.00 is ~990 sigma above mean.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = llmSpan(cost, 100)
	}
	spike := llmSpan(1.00, 100)

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want cost anomaly, got none")
	}
	found := false
	for _, r := range results {
		if r.Metric == "cost_usd" {
			found = true
			if r.Sigma < DefaultConfig().SigmaThreshold {
				t.Errorf("sigma %.2f below threshold %.2f", r.Sigma, DefaultConfig().SigmaThreshold)
			}
			if r.Span.Attributes[AttrAnomaly] != "true" {
				t.Error("AttrAnomaly not set on flagged span")
			}
			if r.Span.Attributes[AttrAnomalyReason] == "" {
				t.Error("AttrAnomalyReason empty on flagged span")
			}
		}
	}
	if !found {
		t.Error("no cost_usd result in anomaly results")
	}
}

func TestDetect_LatencyAnomaly(t *testing.T) {
	// varied baseline so stddev is non-zero: alternating 90ms and 110ms, mean=100ms
	history := make([]storage.Span, 20)
	for i := range history {
		ms := int64(90)
		if i%2 == 0 {
			ms = 110
		}
		history[i] = llmSpan(0.01, ms)
	}
	spike := llmSpan(0.01, 5000) // 5000ms, far above baseline

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Metric == "latency_ms" {
			found = true
		}
	}
	if !found {
		t.Error("want latency anomaly, got none")
	}
}

func TestDetect_BelowThreshold(t *testing.T) {
	// varied baseline: mean=$0.10, stddev=$0.01. $0.11 is 1 sigma, below the 2sigma threshold.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.09
		if i%2 == 0 {
			cost = 0.11
		}
		history[i] = llmSpan(cost, 100)
	}
	normal := llmSpan(0.11, 100) // 1 sigma above mean, not an anomaly

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{normal})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results {
		if r.Metric == "cost_usd" || r.Metric == "latency_ms" {
			t.Errorf("false positive: got anomaly result %+v for normal span", r)
		}
	}
}

func TestDetect_InsufficientHistory(t *testing.T) {
	// fewer than MinSamples — detector must stay silent
	history := baseline(5, 0.01, 100)
	spike := llmSpan(100.0, 100)

	cfg := DefaultConfig()
	d := New(&mockReader{spans: history}, cfg)
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want no anomaly with insufficient history, got %d results", len(results))
	}
}

func TestDetect_NonLLMSpanSkipped(t *testing.T) {
	// agent spans should be ignored entirely
	agentSpan := storage.Span{
		SpanID:    "agent-span",
		ProjectID: "proj",
		Kind:      storage.SpanKindAgent,
		StartTime: time.Now().UTC(),
		Duration:  5000 * time.Millisecond,
	}
	d := New(&mockReader{spans: baseline(20, 0.01, 100)}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{agentSpan})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("non-LLM span should be skipped, got %d results", len(results))
	}
}

func TestDetect_NilGenAISkipped(t *testing.T) {
	span := storage.Span{
		SpanID:    "no-genai",
		ProjectID: "proj",
		Kind:      storage.SpanKindLLM,
		StartTime: time.Now().UTC(),
		Duration:  100 * time.Millisecond,
		GenAI:     nil,
	}
	d := New(&mockReader{spans: baseline(20, 0.01, 100)}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{span})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("nil GenAI span should be skipped, got %d results", len(results))
	}
}

func TestStats(t *testing.T) {
	cases := []struct {
		vals       []float64
		wantMean   float64
		wantStddev float64
	}{
		{[]float64{2, 4, 4, 4, 5, 5, 7, 9}, 5.0, 2.0},
		{[]float64{1, 1, 1, 1}, 1.0, 0.0},
		{[]float64{}, 0.0, 0.0},
	}
	for _, tc := range cases {
		mean, stddev := stats(tc.vals)
		if abs(mean-tc.wantMean) > 1e-9 {
			t.Errorf("stats(%v) mean=%.4f, want %.4f", tc.vals, mean, tc.wantMean)
		}
		if abs(stddev-tc.wantStddev) > 1e-9 {
			t.Errorf("stats(%v) stddev=%.4f, want %.4f", tc.vals, stddev, tc.wantStddev)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
