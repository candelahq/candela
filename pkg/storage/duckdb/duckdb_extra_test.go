package duckdb

// Additional tests for new functionality added in the billing-hardening push:
//   - Environment filter in QueryTraces
//   - OrderBy pushdown (cost, duration, span_count)
//   - GenAI guard with InputTokens/OutputTokens only (no TotalTokens/CostUSD)
//   - GetUserLeaderboard

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── Environment filter ───────────────────────────────────────────────────────

func TestQueryTraces_EnvironmentFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	prod := testSpan("s-prod", "trace-prod", storage.SpanKindLLM, "gpt-4o")
	prod.Environment = "production"
	prod.StartTime = now.Add(-2 * time.Second)
	prod.EndTime = now.Add(-1 * time.Second)

	staging := testSpan("s-stage", "trace-stage", storage.SpanKindLLM, "gpt-4o")
	staging.Environment = "staging"
	staging.StartTime = now.Add(-1 * time.Second)
	staging.EndTime = now

	if err := store.IngestSpans(ctx, []storage.Span{prod, staging}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:   "proj-test",
		Environment: "production",
		StartTime:   now.Add(-10 * time.Second),
		EndTime:     now.Add(10 * time.Second),
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 1 {
		t.Fatalf("got %d traces, want 1 (production only)", len(result.Traces))
	}
	if result.Traces[0].TraceID != "trace-prod" {
		t.Errorf("got trace %q, want trace-prod", result.Traces[0].TraceID)
	}
}

func TestQueryTraces_EmptyEnvironment_ReturnsAll(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	s1 := testSpan("s1", "trace-1", storage.SpanKindLLM, "gpt-4o")
	s1.Environment = "production"
	s1.StartTime = now.Add(-2 * time.Second)

	s2 := testSpan("s2", "trace-2", storage.SpanKindLLM, "gpt-4o")
	s2.Environment = "staging"
	s2.StartTime = now.Add(-1 * time.Second)

	if err := store.IngestSpans(ctx, []storage.Span{s1, s2}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		// Environment intentionally empty → no filter
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 2 {
		t.Errorf("got %d traces, want 2 (all environments)", len(result.Traces))
	}
}

// ─── OrderBy pushdown ─────────────────────────────────────────────────────────

func TestQueryTraces_OrderByTotalCost(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	cheap := testSpan("cheap", "trace-cheap", storage.SpanKindLLM, "gpt-4o-mini")
	cheap.GenAI.CostUSD = 0.001
	cheap.StartTime = now.Add(-2 * time.Second)

	expensive := testSpan("expensive", "trace-expensive", storage.SpanKindLLM, "gpt-4o")
	expensive.GenAI.CostUSD = 0.05
	expensive.StartTime = now.Add(-1 * time.Second)

	if err := store.IngestSpans(ctx, []storage.Span{cheap, expensive}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Ascending by cost → cheap first
	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:  "proj-test",
		StartTime:  now.Add(-10 * time.Second),
		EndTime:    now.Add(10 * time.Second),
		PageSize:   10,
		OrderBy:    "total_cost",
		Descending: false,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 2 {
		t.Fatalf("got %d traces, want 2", len(result.Traces))
	}
	if result.Traces[0].TraceID != "trace-cheap" {
		t.Errorf("first trace = %q, want trace-cheap (cheapest)", result.Traces[0].TraceID)
	}

	// Descending by cost → expensive first
	result2, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:  "proj-test",
		StartTime:  now.Add(-10 * time.Second),
		EndTime:    now.Add(10 * time.Second),
		PageSize:   10,
		OrderBy:    "total_cost",
		Descending: true,
	})
	if err != nil {
		t.Fatalf("query desc: %v", err)
	}
	if result2.Traces[0].TraceID != "trace-expensive" {
		t.Errorf("first trace = %q, want trace-expensive", result2.Traces[0].TraceID)
	}
}

func TestQueryTraces_InvalidOrderBy_DefaultsToTime(t *testing.T) {
	// An unrecognized OrderBy column should not crash or SQL-inject — it falls
	// back to the default MIN(start_time) DESC ordering.
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	s := testSpan("s1", "trace-fallback", storage.SpanKindLLM, "gpt-4o")
	s.StartTime = now
	if err := store.IngestSpans(ctx, []storage.Span{s}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	_, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		OrderBy:   "'; DROP TABLE spans; --", // SQL-injection attempt
	})
	if err != nil {
		t.Errorf("invalid OrderBy should not error: %v", err)
	}
}

// ─── GenAI guard — InputTokens/OutputTokens only ─────────────────────────────

func TestScanSpans_GenAI_PopulatedFromInputOutputOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// A span with no TotalTokens/CostUSD/Model but with meaningful I/O tokens —
	// the expanded GenAI guard should still populate the struct.
	span := storage.Span{
		SpanID:    "span-io",
		TraceID:   "trace-io",
		Name:      "llm.infer",
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(200 * time.Millisecond),
		Duration:  200 * time.Millisecond,
		ProjectID: "proj-test",
		GenAI: &storage.GenAIAttributes{
			Model:        "", // unknown proxied model
			Provider:     "openai",
			InputTokens:  300,
			OutputTokens: 150,
			// TotalTokens and CostUSD intentionally zero
		},
	}

	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "trace-io")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if len(trace.Spans) == 0 {
		t.Fatal("no spans in trace")
	}
	got := trace.Spans[0]
	if got.GenAI == nil {
		t.Fatal("GenAI is nil — guard did not trigger on InputTokens>0")
	}
	if got.GenAI.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", got.GenAI.InputTokens)
	}
	if got.GenAI.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", got.GenAI.OutputTokens)
	}
}

// ─── GetUserLeaderboard ───────────────────────────────────────────────────────

func TestGetUserLeaderboard(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("l1", "trace-l1", storage.SpanKindLLM, "gpt-4o"),
		testSpan("l2", "trace-l2", storage.SpanKindLLM, "gpt-4o"),
		testSpan("l3", "trace-l3", storage.SpanKindLLM, "gpt-4o"),
	}
	spans[0].UserID = "alice@example.com"
	spans[0].GenAI.CostUSD = 0.05
	spans[1].UserID = "alice@example.com"
	spans[1].GenAI.CostUSD = 0.03
	spans[2].UserID = "bob@example.com"
	spans[2].GenAI.CostUSD = 0.10
	for i := range spans {
		spans[i].StartTime = now.Add(time.Duration(-i) * time.Second)
		spans[i].EndTime = spans[i].StartTime.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	leaders, err := store.GetUserLeaderboard(ctx, storage.UsageQuery{
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
	// Bob should be first (highest cost $0.10)
	if leaders[0].UserID != "bob@example.com" {
		t.Errorf("top user = %q, want bob@example.com", leaders[0].UserID)
	}
	// Alice has 2 calls totalling $0.08
	if leaders[1].UserID != "alice@example.com" {
		t.Errorf("second user = %q, want alice@example.com", leaders[1].UserID)
	}
	if leaders[1].CallCount != 2 {
		t.Errorf("alice call count = %d, want 2", leaders[1].CallCount)
	}
}
