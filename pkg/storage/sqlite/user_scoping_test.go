package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// testSpanWithUser creates a test span with a specific user_id.
func testSpanWithUser(id, traceID, userID string) storage.Span {
	now := time.Now().Truncate(time.Millisecond)
	return storage.Span{
		SpanID:      id,
		TraceID:     traceID,
		Name:        "test." + id,
		Kind:        storage.SpanKindLLM,
		Status:      storage.SpanStatusOK,
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
		Duration:    100 * time.Millisecond,
		ProjectID:   "proj-test",
		Environment: "test",
		UserID:      userID,
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.001,
		},
	}
}

func newTestSQLiteStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestQueryTraces_UserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "trace-alice", "alice@example.com"),
		testSpanWithUser("s2", "trace-bob", "bob@example.com"),
		testSpanWithUser("s3", "trace-alice2", "alice@example.com"),
	}
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

	t.Run("admin sees all", func(t *testing.T) {
		q := timeRange
		q.UserID = ""
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 3 {
			t.Errorf("trace count = %d, want 3", len(result.Traces))
		}
	})

	t.Run("alice sees 2", func(t *testing.T) {
		q := timeRange
		q.UserID = "alice@example.com"
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 2 {
			t.Errorf("trace count = %d, want 2", len(result.Traces))
		}
	})

	t.Run("bob sees 1", func(t *testing.T) {
		q := timeRange
		q.UserID = "bob@example.com"
		result, err := store.QueryTraces(ctx, q)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 1 {
			t.Errorf("trace count = %d, want 1", len(result.Traces))
		}
	})
}

func TestSearchSpans_UserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com"),
		testSpanWithUser("s2", "t2", "bob@example.com"),
	}
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	t.Run("alice sees only her spans", func(t *testing.T) {
		result, err := store.SearchSpans(ctx, storage.SpanQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "alice@example.com",
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 1 {
			t.Errorf("span count = %d, want 1", len(result.Spans))
		}
	})

	t.Run("admin sees all spans", func(t *testing.T) {
		result, err := store.SearchSpans(ctx, storage.SpanQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "",
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(result.Spans) != 2 {
			t.Errorf("span count = %d, want 2", len(result.Spans))
		}
	})
}

func TestGetUsageSummary_UserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com"),
		testSpanWithUser("s2", "t2", "bob@example.com"),
	}
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	t.Run("admin sees all", func(t *testing.T) {
		summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "",
		})
		if err != nil {
			t.Fatalf("usage: %v", err)
		}
		if summary.TotalSpans != 2 {
			t.Errorf("TotalSpans = %d, want 2", summary.TotalSpans)
		}
	})

	t.Run("alice sees only hers", func(t *testing.T) {
		summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "alice@example.com",
		})
		if err != nil {
			t.Fatalf("usage: %v", err)
		}
		if summary.TotalSpans != 1 {
			t.Errorf("TotalSpans = %d, want 1", summary.TotalSpans)
		}
	})
}

func TestGetModelBreakdown_UserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Millisecond)

	spans := []storage.Span{
		testSpanWithUser("s1", "t1", "alice@example.com"),
		testSpanWithUser("s2", "t2", "bob@example.com"),
	}
	spans[1].GenAI.Model = "gemini-2.0"
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	t.Run("alice sees only gpt-4o", func(t *testing.T) {
		models, err := store.GetModelBreakdown(ctx, storage.UsageQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "alice@example.com",
		})
		if err != nil {
			t.Fatalf("model breakdown: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("model count = %d, want 1", len(models))
		}
		if models[0].Model != "gpt-4o" {
			t.Errorf("model = %q, want gpt-4o", models[0].Model)
		}
	})

	t.Run("admin sees both models", func(t *testing.T) {
		models, err := store.GetModelBreakdown(ctx, storage.UsageQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-10 * time.Second),
			EndTime:   now.Add(10 * time.Second),
			UserID:    "",
		})
		if err != nil {
			t.Fatalf("model breakdown: %v", err)
		}
		if len(models) != 2 {
			t.Errorf("model count = %d, want 2", len(models))
		}
	})
}
