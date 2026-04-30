package duckdb

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// testSpanWithUser creates a test span with a specific user_id.
func testSpanWithUser(id, traceID, userID string, kind storage.SpanKind, model string) storage.Span {
	sp := testSpan(id, traceID, kind, model)
	sp.UserID = userID
	return sp
}

// TestGetTrace_UserID_RoundTrip verifies that GetTrace returns spans with
// UserID populated. This is critical: the handler's authorization gate in
// traceUserID() relies on this field being set in the returned spans.
func TestGetTrace_UserID_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := testSpanWithUser("s1", "t1", "alice@example.com", storage.SpanKindLLM, "gpt-4o")
	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "t1")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(trace.Spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(trace.Spans))
	}
	if trace.Spans[0].UserID != "alice@example.com" {
		t.Errorf("UserID = %q, want %q (auth gate depends on this!)",
			trace.Spans[0].UserID, "alice@example.com")
	}
}

func TestQueryTraces_UserScoping(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "trace-alice", "alice@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s2", "trace-bob", "bob@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s3", "trace-alice2", "alice@example.com", storage.SpanKindLLM, "gemini-2.0"),
	}
	// Spread in time for deterministic ordering.
	spans[0].StartTime = now.Add(-3 * time.Second)
	spans[0].EndTime = now.Add(-2 * time.Second)
	spans[1].StartTime = now.Add(-2 * time.Second)
	spans[1].EndTime = now.Add(-1 * time.Second)
	spans[2].StartTime = now.Add(-1 * time.Second)
	spans[2].EndTime = now

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	timeRange := storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  50,
	}

	t.Run("admin sees all traces", func(t *testing.T) {
		q := timeRange
		q.UserID = "" // empty = admin
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 3 {
			t.Errorf("trace count = %d, want 3 (admin sees all)", len(result.Traces))
		}
	})

	t.Run("alice sees only her traces", func(t *testing.T) {
		q := timeRange
		q.UserID = "alice@example.com"
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 2 {
			t.Fatalf("trace count = %d, want 2 (alice's traces)", len(result.Traces))
		}
		for _, tr := range result.Traces {
			if tr.TraceID != "trace-alice" && tr.TraceID != "trace-alice2" {
				t.Errorf("unexpected trace %q in alice's results", tr.TraceID)
			}
		}
	})

	t.Run("bob sees only his trace", func(t *testing.T) {
		q := timeRange
		q.UserID = "bob@example.com"
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 1 {
			t.Fatalf("trace count = %d, want 1 (bob's trace)", len(result.Traces))
		}
		if result.Traces[0].TraceID != "trace-bob" {
			t.Errorf("trace = %q, want trace-bob", result.Traces[0].TraceID)
		}
	})

	t.Run("unknown user sees nothing", func(t *testing.T) {
		q := timeRange
		q.UserID = "nobody@example.com"
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("trace count = %d, want 0 (unknown user)", len(result.Traces))
		}
	})
}

func TestSearchSpans_UserScoping(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s2", "t2", "bob@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s3", "t3", "alice@example.com", storage.SpanKindTool, ""),
	}
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	baseQ := storage.SpanQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}

	t.Run("admin sees all spans", func(t *testing.T) {
		q := baseQ
		q.UserID = ""
		result, err := store.SearchSpans(ctx, q)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 3 {
			t.Errorf("span count = %d, want 3", len(result.Spans))
		}
	})

	t.Run("alice sees only her spans", func(t *testing.T) {
		q := baseQ
		q.UserID = "alice@example.com"
		result, err := store.SearchSpans(ctx, q)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 2 {
			t.Errorf("span count = %d, want 2 (alice's spans)", len(result.Spans))
		}
	})

	t.Run("bob sees only his span", func(t *testing.T) {
		q := baseQ
		q.UserID = "bob@example.com"
		result, err := store.SearchSpans(ctx, q)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 1 {
			t.Errorf("span count = %d, want 1 (bob's span)", len(result.Spans))
		}
	})

	t.Run("user scoping combines with kind filter", func(t *testing.T) {
		q := baseQ
		q.UserID = "alice@example.com"
		q.Kind = storage.SpanKindLLM
		result, err := store.SearchSpans(ctx, q)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 1 {
			t.Errorf("span count = %d, want 1 (alice's LLM span only)", len(result.Spans))
		}
	})
}

func TestGetUsageSummary_UserScoping(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s2", "t2", "bob@example.com", storage.SpanKindLLM, "gpt-4o"),
	}
	// Give bob a different cost.
	spans[1].GenAI.CostUSD = 0.005
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	baseQ := storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}

	t.Run("admin sees total cost", func(t *testing.T) {
		q := baseQ
		q.UserID = ""
		summary, err := store.GetUsageSummary(ctx, q)
		if err != nil {
			t.Fatalf("usage: %v", err)
		}
		if summary.TotalSpans != 2 {
			t.Errorf("TotalSpans = %d, want 2", summary.TotalSpans)
		}
	})

	t.Run("alice sees only her usage", func(t *testing.T) {
		q := baseQ
		q.UserID = "alice@example.com"
		summary, err := store.GetUsageSummary(ctx, q)
		if err != nil {
			t.Fatalf("usage: %v", err)
		}
		if summary.TotalSpans != 1 {
			t.Errorf("TotalSpans = %d, want 1", summary.TotalSpans)
		}
		if summary.TotalCostUSD < 0.0009 || summary.TotalCostUSD > 0.0011 {
			t.Errorf("TotalCostUSD = %f, want ~0.001 (alice only)", summary.TotalCostUSD)
		}
	})
}

func TestGetModelBreakdown_UserScoping(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com", storage.SpanKindLLM, "gpt-4o"),
		testSpanWithUser("s2", "t2", "bob@example.com", storage.SpanKindLLM, "gemini-2.0"),
		testSpanWithUser("s3", "t3", "alice@example.com", storage.SpanKindLLM, "gemini-2.0"),
	}
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	baseQ := storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}

	t.Run("admin sees all models", func(t *testing.T) {
		q := baseQ
		q.UserID = ""
		models, err := store.GetModelBreakdown(ctx, q)
		if err != nil {
			t.Fatalf("model breakdown: %v", err)
		}
		if len(models) != 2 {
			t.Errorf("model count = %d, want 2 (gpt-4o + gemini-2.0)", len(models))
		}
	})

	t.Run("alice sees her models", func(t *testing.T) {
		q := baseQ
		q.UserID = "alice@example.com"
		models, err := store.GetModelBreakdown(ctx, q)
		if err != nil {
			t.Fatalf("model breakdown: %v", err)
		}
		if len(models) != 2 {
			t.Errorf("model count = %d, want 2 (alice uses both models)", len(models))
		}
		byModel := make(map[string]storage.ModelUsage)
		for _, m := range models {
			byModel[m.Model] = m
		}
		if byModel["gpt-4o"].CallCount != 1 {
			t.Errorf("alice gpt-4o calls = %d, want 1", byModel["gpt-4o"].CallCount)
		}
	})

	t.Run("bob sees only gemini", func(t *testing.T) {
		q := baseQ
		q.UserID = "bob@example.com"
		models, err := store.GetModelBreakdown(ctx, q)
		if err != nil {
			t.Fatalf("model breakdown: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("model count = %d, want 1 (bob only uses gemini)", len(models))
		}
		if models[0].Model != "gemini-2.0" {
			t.Errorf("model = %q, want gemini-2.0", models[0].Model)
		}
	})
}
