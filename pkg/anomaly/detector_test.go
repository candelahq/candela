package anomaly

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

// --- Additional edge-case and gap-coverage tests ---

// errReader is a SpanReader stub that returns a fixed error from SearchSpans.
type errReader struct {
	mockReader
	searchErr error
}

func (e *errReader) SearchSpans(_ context.Context, _ storage.SpanQuery) (*storage.SpanResult, error) {
	return nil, e.searchErr
}

// queryCapturingReader records the SpanQuery arguments passed to SearchSpans
// so tests can assert on grouping and anchor-time logic.
type queryCapturingReader struct {
	mockReader
	mu      sync.Mutex
	queries []storage.SpanQuery
}

func (q *queryCapturingReader) SearchSpans(_ context.Context, query storage.SpanQuery) (*storage.SpanResult, error) {
	q.mu.Lock()
	q.queries = append(q.queries, query)
	q.mu.Unlock()
	return q.mockReader.SearchSpans(context.Background(), query)
}

func TestDetect_SearchSpansError(t *testing.T) {
	// When the reader returns an error, Detect must propagate it.
	reader := &errReader{searchErr: fmt.Errorf("connection refused")}
	d := New(reader, DefaultConfig())

	spike := llmSpan(1.00, 100)
	_, err := d.Detect(context.Background(), []storage.Span{spike})
	if err == nil {
		t.Fatal("expected error from Detect when SearchSpans fails, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to contain 'connection refused'", err)
	}
}

func TestDetect_MultipleGroups(t *testing.T) {
	// Spans from two different (tenant, model) groups should each be queried
	// separately and anomalies detected independently.
	now := time.Now().UTC()

	// Varied baseline for both groups — alternating costs so stddev is non-zero.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = llmSpan(cost, 100)
	}

	reader := &queryCapturingReader{mockReader: mockReader{spans: history}}
	d := New(reader, DefaultConfig())

	// Two spikes from different tenants / models.
	spikeA := storage.Span{
		SpanID:    "spike-a",
		ProjectID: "proj",
		TenantID:  "tenant-alpha",
		Kind:      storage.SpanKindLLM,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", CostUSD: 5.00},
	}
	spikeB := storage.Span{
		SpanID:    "spike-b",
		ProjectID: "proj",
		TenantID:  "tenant-beta",
		Kind:      storage.SpanKindLLM,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		GenAI:     &storage.GenAIAttributes{Model: "claude-3", CostUSD: 10.00},
	}

	results, err := d.Detect(context.Background(), []storage.Span{spikeA, spikeB})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	// Should have issued exactly 2 SearchSpans queries (one per group).
	reader.mu.Lock()
	numQueries := len(reader.queries)
	reader.mu.Unlock()
	if numQueries != 2 {
		t.Errorf("expected 2 SearchSpans queries (one per group), got %d", numQueries)
	}

	// Both spikes should be flagged as cost anomalies.
	spanIDs := make(map[string]bool)
	for _, r := range results {
		if r.Metric == "cost_usd" {
			spanIDs[r.Span.SpanID] = true
		}
	}
	if !spanIDs["spike-a"] {
		t.Error("spike-a not flagged as cost anomaly")
	}
	if !spanIDs["spike-b"] {
		t.Error("spike-b not flagged as cost anomaly")
	}
}

func TestDetect_RelativeDeltaGuard(t *testing.T) {
	// The 10% relative-delta guard should suppress anomalies where the observed
	// value is statistically >2 sigma but the absolute difference from the mean
	// is <10% of the mean. This happens when stddev is very small.
	//
	// Build a baseline where mean=100.0 and stddev is very small (~0.5),
	// then check a value that's >2sigma but <10% above mean.
	history := make([]storage.Span, 20)
	for i := range history {
		// Costs alternating between 99.5 and 100.5 → mean=100, stddev=0.5
		cost := 99.5
		if i%2 == 0 {
			cost = 100.5
		}
		history[i] = llmSpan(cost, 100)
	}
	// observed=101.5 → sigma=(101.5-100)/0.5=3.0 which is >2sigma threshold,
	// but (101.5-100)/100 = 0.015 which is < 0.10 → should be filtered.
	borderline := llmSpan(101.5, 100)

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{borderline})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results {
		if r.Metric == "cost_usd" {
			t.Errorf("expected 10%% relative-delta guard to suppress cost anomaly, got sigma=%.2f, delta=%.4f%%",
				r.Sigma, (r.Value-r.Mean)/r.Mean*100)
		}
	}
}

func TestDetect_NearZeroStddevGuard(t *testing.T) {
	// When all historical values are nearly identical (stddev/mean < 0.001),
	// the detector should suppress anomalies to avoid false positives.
	// This is different from stddev==0 (which the existing tests cover via baseline()).
	history := make([]storage.Span, 20)
	for i := range history {
		// Very tight spread: alternating 10.000 and 10.001
		// mean ~= 10.0005, stddev ~= 0.0005, stddev/mean ~= 0.00005 < 0.001
		cost := 10.000
		if i%2 == 0 {
			cost = 10.001
		}
		history[i] = llmSpan(cost, 100)
	}
	// Even though sigma would be astronomically high, the near-zero guard should block it.
	spike := llmSpan(10.01, 100)

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results {
		if r.Metric == "cost_usd" {
			t.Error("near-zero stddev guard should suppress cost anomaly for nearly-identical baseline")
		}
	}
}

func TestFormatSigma(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{2.0, "2"},
		{3.0, "3"},
		{1.5, "1.5"},
		{2.5, "2.5"},
		{0.0, "0"},
		{10.0, "10"},
	}
	for _, tc := range cases {
		got := formatSigma(tc.input)
		if got != tc.want {
			t.Errorf("formatSigma(%.1f) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDetect_CustomSigmaThreshold(t *testing.T) {
	// With a very high sigma threshold (e.g. 10), a moderate spike should not
	// be flagged even though it would be flagged at the default 2-sigma.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = llmSpan(cost, 100)
	}
	// $0.05 is ~40 sigma above mean=$0.01 with stddev=$0.001, well above 2 sigma
	// but let's use threshold=100 to ensure it's not flagged.
	moderateSpike := llmSpan(0.05, 100)

	cfg := Config{
		WindowDays:     7,
		SigmaThreshold: 100.0, // very high threshold
		MinSamples:     10,
	}
	d := New(&mockReader{spans: history}, cfg)
	results, err := d.Detect(context.Background(), []storage.Span{moderateSpike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results {
		if r.Metric == "cost_usd" {
			t.Errorf("sigma=%.2f should be below threshold=100, but got flagged", r.Sigma)
		}
	}
}

func TestDetect_EmptyBatch(t *testing.T) {
	// Empty batch should return nil results, no error, and no SearchSpans calls.
	reader := &queryCapturingReader{mockReader: mockReader{spans: baseline(20, 0.01, 100)}}
	d := New(reader, DefaultConfig())

	results, err := d.Detect(context.Background(), []storage.Span{})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results for empty batch, got %d", len(results))
	}
	reader.mu.Lock()
	numQueries := len(reader.queries)
	reader.mu.Unlock()
	if numQueries != 0 {
		t.Errorf("expected 0 SearchSpans queries for empty batch, got %d", numQueries)
	}
}

func TestDetect_AnchorTimeUsesEarliestSpan(t *testing.T) {
	// When a group has multiple spans, the window anchor should be based on
	// the earliest StartTime, not the first span in the slice.
	now := time.Now().UTC()
	earlier := now.Add(-1 * time.Hour)

	span1 := storage.Span{
		SpanID:    "span-later",
		ProjectID: "proj",
		Kind:      storage.SpanKindLLM,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", CostUSD: 5.00},
	}
	span2 := storage.Span{
		SpanID:    "span-earlier",
		ProjectID: "proj",
		Kind:      storage.SpanKindLLM,
		StartTime: earlier,
		EndTime:   earlier.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", CostUSD: 5.00},
	}

	// Varied baseline to avoid near-zero stddev guard
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = llmSpan(cost, 100)
	}

	reader := &queryCapturingReader{mockReader: mockReader{spans: history}}
	d := New(reader, DefaultConfig())

	// Submit span1 first (later time), then span2 (earlier time).
	_, err := d.Detect(context.Background(), []storage.Span{span1, span2})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(reader.queries))
	}
	// The EndTime of the query should be the earlier span's StartTime.
	if !reader.queries[0].EndTime.Equal(earlier) {
		t.Errorf("query EndTime = %v, want %v (earliest span's StartTime)", reader.queries[0].EndTime, earlier)
	}
}

func TestDetect_BothMetricsFlaggedSimultaneously(t *testing.T) {
	// A span that's anomalous in BOTH cost and latency should produce two Results.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		ms := int64(90)
		if i%2 == 0 {
			cost = 0.011
			ms = 110
		}
		history[i] = llmSpan(cost, ms)
	}
	// Both cost ($5.00 vs $0.01) and latency (10000ms vs 100ms) are massive spikes.
	spike := llmSpan(5.00, 10000)

	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	metrics := make(map[string]bool)
	for _, r := range results {
		metrics[r.Metric] = true
	}
	if !metrics["cost_usd"] {
		t.Error("expected cost_usd anomaly")
	}
	if !metrics["latency_ms"] {
		t.Error("expected latency_ms anomaly")
	}
}

func TestDetect_ReasonAttributeFormat(t *testing.T) {
	// Verify the anomaly reason attribute format matches the expected pattern.
	history := make([]storage.Span, 20)
	for i := range history {
		cost := 0.009
		if i%2 == 0 {
			cost = 0.011
		}
		history[i] = llmSpan(cost, 100)
	}
	spike := llmSpan(1.00, 100)

	// Test with default 2-sigma threshold.
	d := New(&mockReader{spans: history}, DefaultConfig())
	results, err := d.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results {
		if r.Metric == "cost_usd" {
			want := "cost_usd_2sigma"
			got := r.Span.Attributes[AttrAnomalyReason]
			if got != want {
				t.Errorf("AttrAnomalyReason = %q, want %q", got, want)
			}
		}
	}

	// Test with fractional sigma threshold (1.5).
	cfg := Config{WindowDays: 7, SigmaThreshold: 1.5, MinSamples: 10}
	d2 := New(&mockReader{spans: history}, cfg)
	results2, err := d2.Detect(context.Background(), []storage.Span{spike})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, r := range results2 {
		if r.Metric == "cost_usd" {
			want := "cost_usd_1.5sigma"
			got := r.Span.Attributes[AttrAnomalyReason]
			if got != want {
				t.Errorf("AttrAnomalyReason = %q, want %q", got, want)
			}
		}
	}
}

func BenchmarkDetect(b *testing.B) {
	// Benchmark detection with a realistic batch size and history.
	history := make([]storage.Span, 1000)
	for i := range history {
		cost := 0.009 + float64(i%5)*0.001
		history[i] = llmSpan(cost, int64(90+i%20))
	}
	batch := make([]storage.Span, 50)
	for i := range batch {
		batch[i] = llmSpan(0.01, 100)
	}

	d := New(&mockReader{spans: history}, DefaultConfig())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, batch)
	}
}
