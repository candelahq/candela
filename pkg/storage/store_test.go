package storage

import (
	"testing"
	"time"
)

func TestSpanKindValues(t *testing.T) {
	// Ensure SpanKind constants align with proto enum values.
	tests := []struct {
		kind SpanKind
		want int
	}{
		{SpanKindUnspecified, 0},
		{SpanKindLLM, 1},
		{SpanKindAgent, 2},
		{SpanKindTool, 3},
		{SpanKindRetrieval, 4},
		{SpanKindEmbedding, 5},
		{SpanKindChain, 6},
		{SpanKindGeneral, 7},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("SpanKind(%d) = %d, want %d", tt.kind, int(tt.kind), tt.want)
		}
	}
}

func TestSpanStatusValues(t *testing.T) {
	tests := []struct {
		status SpanStatus
		want   int
	}{
		{SpanStatusUnspecified, 0},
		{SpanStatusOK, 1},
		{SpanStatusError, 2},
	}
	for _, tt := range tests {
		if int(tt.status) != tt.want {
			t.Errorf("SpanStatus(%d) = %d, want %d", tt.status, int(tt.status), tt.want)
		}
	}
}

func TestTraceQueryDefaults(t *testing.T) {
	q := TraceQuery{}

	if q.PageSize != 0 {
		t.Errorf("default PageSize = %d, want 0", q.PageSize)
	}
	if q.Descending != false {
		t.Errorf("default Descending = %v, want false", q.Descending)
	}
	if !q.StartTime.IsZero() {
		t.Errorf("default StartTime should be zero")
	}
}

func TestSpanDuration(t *testing.T) {
	now := time.Now()
	span := Span{
		SpanID:    "abc123",
		TraceID:   "trace-001",
		Name:      "llm.chat",
		Kind:      SpanKindLLM,
		Status:    SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(150 * time.Millisecond),
		Duration:  150 * time.Millisecond,
		GenAI: &GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.00075,
		},
		ProjectID: "proj-1",
	}

	if span.Duration != 150*time.Millisecond {
		t.Errorf("Duration = %v, want 150ms", span.Duration)
	}
	if span.GenAI.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", span.GenAI.TotalTokens)
	}
	if span.Kind != SpanKindLLM {
		t.Errorf("Kind = %d, want SpanKindLLM(%d)", span.Kind, SpanKindLLM)
	}
}

func TestTraceSummaryFields(t *testing.T) {
	ts := TraceSummary{
		TraceID:         "trace-001",
		RootSpanName:    "agent.run",
		SpanCount:       12,
		LLMCallCount:    3,
		TotalTokens:     4500,
		TotalCostUSD:    0.045,
		Status:          SpanStatusOK,
		PrimaryModel:    "gemini-2.0-flash",
		PrimaryProvider: "google",
	}

	if ts.SpanCount != 12 {
		t.Errorf("SpanCount = %d, want 12", ts.SpanCount)
	}
	if ts.LLMCallCount != 3 {
		t.Errorf("LLMCallCount = %d, want 3", ts.LLMCallCount)
	}
	if ts.TotalCostUSD < 0.04 || ts.TotalCostUSD > 0.05 {
		t.Errorf("TotalCostUSD = %f, want ~0.045", ts.TotalCostUSD)
	}
}

func TestGenAIAttributesZeroValue(t *testing.T) {
	// A span without GenAI attributes should have nil GenAI.
	span := Span{
		SpanID: "span-http",
		Kind:   SpanKindGeneral,
	}

	if span.GenAI != nil {
		t.Error("GenAI should be nil for general spans")
	}
}

func TestTraceAggregation(t *testing.T) {
	now := time.Now()
	trace := Trace{
		TraceID:   "trace-agg",
		StartTime: now,
		EndTime:   now.Add(2 * time.Second),
		Duration:  2 * time.Second,
		Spans: []Span{
			{
				SpanID: "root", Kind: SpanKindAgent,
				GenAI: &GenAIAttributes{TotalTokens: 0},
			},
			{
				SpanID: "llm-1", Kind: SpanKindLLM,
				GenAI: &GenAIAttributes{TotalTokens: 1000, CostUSD: 0.01},
			},
			{
				SpanID: "llm-2", Kind: SpanKindLLM,
				GenAI: &GenAIAttributes{TotalTokens: 2000, CostUSD: 0.02},
			},
			{
				SpanID: "tool-1", Kind: SpanKindTool,
			},
		},
	}

	// Verify we can compute aggregates from spans.
	var totalTokens int64
	var totalCost float64
	var llmCount int
	for _, s := range trace.Spans {
		if s.GenAI != nil {
			totalTokens += s.GenAI.TotalTokens
			totalCost += s.GenAI.CostUSD
		}
		if s.Kind == SpanKindLLM {
			llmCount++
		}
	}

	if totalTokens != 3000 {
		t.Errorf("totalTokens = %d, want 3000", totalTokens)
	}
	if totalCost < 0.029 || totalCost > 0.031 {
		t.Errorf("totalCost = %f, want ~0.03", totalCost)
	}
	if llmCount != 2 {
		t.Errorf("llmCount = %d, want 2", llmCount)
	}
	if len(trace.Spans) != 4 {
		t.Errorf("span count = %d, want 4", len(trace.Spans))
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"user_name", `user\_name`},
		{`path\to`, `path\\to`},
		{"100%_done", `100\%\_done`},
		{"", ""},
	}
	for _, tt := range tests {
		got := EscapeLike(tt.input)
		if got != tt.want {
			t.Errorf("EscapeLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
