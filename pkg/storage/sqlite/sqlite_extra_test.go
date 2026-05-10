package sqlite

// Extra tests for new functionality: environment filter, OrderBy, GenAI guard.

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func spanWithEnv(id, traceID, env string, cost float64, start time.Time) storage.Span {
	return storage.Span{
		SpanID:      id,
		TraceID:     traceID,
		Name:        "llm.chat",
		Kind:        storage.SpanKindLLM,
		Status:      storage.SpanStatusOK,
		StartTime:   start,
		EndTime:     start.Add(100 * time.Millisecond),
		Duration:    100 * time.Millisecond,
		ProjectID:   "proj-test",
		Environment: env,
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      cost,
		},
	}
}

func TestQueryTraces_EnvironmentFilter_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		spanWithEnv("p1", "trace-prod", "production", 0.01, now.Add(-2*time.Second)),
		spanWithEnv("s1", "trace-stage", "staging", 0.02, now.Add(-1*time.Second)),
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:   "proj-test",
		Environment: "staging",
		StartTime:   now.Add(-10 * time.Second),
		EndTime:     now.Add(10 * time.Second),
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 1 {
		t.Fatalf("got %d traces, want 1 (staging only)", len(result.Traces))
	}
	if result.Traces[0].TraceID != "trace-stage" {
		t.Errorf("trace = %q, want trace-stage", result.Traces[0].TraceID)
	}
}

func TestQueryTraces_OrderByCost_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		spanWithEnv("c1", "trace-cheap", "test", 0.001, now.Add(-2*time.Second)),
		spanWithEnv("c2", "trace-expensive", "test", 0.10, now.Add(-1*time.Second)),
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:  "proj-test",
		StartTime:  now.Add(-10 * time.Second),
		EndTime:    now.Add(10 * time.Second),
		PageSize:   10,
		OrderBy:    "total_cost",
		Descending: true,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 2 {
		t.Fatalf("got %d traces, want 2", len(result.Traces))
	}
	if result.Traces[0].TraceID != "trace-expensive" {
		t.Errorf("first = %q, want trace-expensive", result.Traces[0].TraceID)
	}
}

func TestQueryTraces_DefaultSortIsDescTime_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		spanWithEnv("old", "trace-old", "test", 0.01, now.Add(-3*time.Second)),
		spanWithEnv("new", "trace-new", "test", 0.01, now.Add(-1*time.Second)),
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		// No OrderBy → default DESC by start_time
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 2 {
		t.Fatalf("got %d, want 2", len(result.Traces))
	}
	if result.Traces[0].TraceID != "trace-new" {
		t.Errorf("first = %q, want trace-new (most recent)", result.Traces[0].TraceID)
	}
}

func TestGetUserLeaderboard_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		spanWithEnv("l1", "trace-l1", "test", 0.10, now.Add(-3*time.Second)),
		spanWithEnv("l2", "trace-l2", "test", 0.05, now.Add(-2*time.Second)),
		spanWithEnv("l3", "trace-l3", "test", 0.05, now.Add(-1*time.Second)),
	}
	spans[0].UserID = "bob@example.com"
	spans[1].UserID = "alice@example.com"
	spans[2].UserID = "alice@example.com"

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	leaders, err := s.GetUserLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}, 10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(leaders) != 2 {
		t.Fatalf("got %d leaders, want 2", len(leaders))
	}
	// Bob: $0.10 total → first
	if leaders[0].UserID != "bob@example.com" {
		t.Errorf("top user = %q, want bob@example.com", leaders[0].UserID)
	}
	// Alice: 2 calls, $0.10 total → second
	if leaders[1].UserID != "alice@example.com" {
		t.Errorf("second user = %q, want alice@example.com", leaders[1].UserID)
	}
	if leaders[1].CallCount != 2 {
		t.Errorf("alice call count = %d, want 2", leaders[1].CallCount)
	}
}

func TestScanSpans_GenAI_InputOutputOnly_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	span := storage.Span{
		SpanID:    "io-only",
		TraceID:   "trace-io",
		Name:      "llm.proxy",
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		ProjectID: "proj-test",
		GenAI: &storage.GenAIAttributes{
			Model:        "", // unknown model — proxied traffic
			Provider:     "anthropic",
			InputTokens:  500,
			OutputTokens: 200,
			// TotalTokens and CostUSD are zero
		},
	}
	if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := s.GetTrace(ctx, "trace-io")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if len(trace.Spans) == 0 {
		t.Fatal("no spans returned")
	}
	got := trace.Spans[0]
	if got.GenAI == nil {
		t.Fatal("GenAI is nil — guard did not fire for InputTokens>0")
	}
	if got.GenAI.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", got.GenAI.InputTokens)
	}
}
