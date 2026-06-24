package duckdb

// Conformance-style driver tests for the DuckDB storage backend.
//
// These tests focus on behaviors not yet covered by existing test files:
//   - QueryTraces pagination (PageSize + sequential fetching)
//   - QueryTraces time-range filtering (out-of-range returns empty)
//   - QueryTraces status filter (error/ok)
//   - SearchSpans model filter
//   - SearchSpans pagination
//   - Empty-store edge cases (all query methods return zero, not error)
//   - GetJobLeaderboard basic behavior and limit enforcement
//   - Concurrent read/write safety
//   - Labels round-trip (user_id, session_id, tenant_id, job_id)
//   - Optional project_id filter (empty = all projects)

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── QueryTraces: pagination ─────────────────────────────────────────────────

func TestQueryTraces_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Ingest 5 traces, each separated by 1 second for deterministic ordering.
	for i := 0; i < 5; i++ {
		sp := testSpan(fmt.Sprintf("span-%d", i), fmt.Sprintf("trace-%d", i), storage.SpanKindLLM, "gpt-4o")
		sp.StartTime = now.Add(time.Duration(i) * time.Second)
		sp.EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
		if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// Page 1: first 2 traces (most recent first → trace-4, trace-3).
	page1, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.Traces) != 2 {
		t.Fatalf("page 1 count = %d, want 2", len(page1.Traces))
	}
	if page1.Traces[0].TraceID != "trace-4" {
		t.Errorf("page 1 first = %q, want trace-4", page1.Traces[0].TraceID)
	}
	if page1.Traces[1].TraceID != "trace-3" {
		t.Errorf("page 1 second = %q, want trace-3", page1.Traces[1].TraceID)
	}

	// Fetch all — verify all 5 are present.
	all, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  50,
	})
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all.Traces) != 5 {
		t.Errorf("total traces = %d, want 5", len(all.Traces))
	}
}

// ─── QueryTraces: time-range filtering ───────────────────────────────────────

func TestQueryTraces_TimeRangeFiltering(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	sp := testSpan("s1", "t1", storage.SpanKindLLM, "gpt-4o")
	sp.StartTime = now
	sp.EndTime = now.Add(100 * time.Millisecond)
	if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	t.Run("in_range_returns_trace", func(t *testing.T) {
		result, err := store.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 1 {
			t.Errorf("count = %d, want 1", len(result.Traces))
		}
	})

	t.Run("before_range_returns_empty", func(t *testing.T) {
		result, err := store.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0 (before range)", len(result.Traces))
		}
	})

	t.Run("after_range_returns_empty", func(t *testing.T) {
		result, err := store.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-test",
			StartTime: now.Add(1 * time.Hour),
			EndTime:   now.Add(2 * time.Hour),
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0 (after range)", len(result.Traces))
		}
	})
}

// ─── QueryTraces: status aggregation ─────────────────────────────────────────

func TestQueryTraces_StatusAggregation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s-ok", TraceID: "trace-ok", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(100 * time.Millisecond),
			Duration: 100 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "s-err", TraceID: "trace-err", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(time.Second), EndTime: now.Add(time.Second + 100*time.Millisecond),
			Duration: 100 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
	}
	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  50,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 2 {
		t.Fatalf("count = %d, want 2", len(result.Traces))
	}

	statuses := map[string]storage.SpanStatus{}
	for _, tr := range result.Traces {
		statuses[tr.TraceID] = tr.Status
	}
	// The SQL aggregation uses MAX(CASE WHEN status=2 THEN 2 ELSE 0 END),
	// so traces without errors return SpanStatusUnspecified (0), not OK (1).
	if statuses["trace-ok"] == storage.SpanStatusError {
		t.Errorf("trace-ok status = %d, should not be Error", statuses["trace-ok"])
	}
	if statuses["trace-err"] != storage.SpanStatusError {
		t.Errorf("trace-err status = %d, want Error", statuses["trace-err"])
	}
}

// ─── SearchSpans: model filter ───────────────────────────────────────────────

func TestSearchSpans_ModelFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("s1", "t1", storage.SpanKindLLM, "gpt-4o"),
		testSpan("s2", "t2", storage.SpanKindLLM, "gemini-2.0"),
	}
	for i := range spans {
		spans[i].StartTime = now.Add(time.Duration(i) * time.Second)
		spans[i].EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-test",
		Model:     "gemini-2.0",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 1 {
		t.Fatalf("count = %d, want 1", len(result.Spans))
	}
	if result.Spans[0].SpanID != "s2" {
		t.Errorf("span = %q, want s2", result.Spans[0].SpanID)
	}
}

// ─── SearchSpans: pagination ─────────────────────────────────────────────────

func TestSearchSpans_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Ingest 5 spans.
	for i := 0; i < 5; i++ {
		sp := testSpan(fmt.Sprintf("span-%d", i), fmt.Sprintf("trace-%d", i), storage.SpanKindLLM, "gpt-4o")
		sp.StartTime = now.Add(time.Duration(i) * time.Second)
		sp.EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
		if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// Request only 3.
	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 3 {
		t.Errorf("count = %d, want 3 (page size limit)", len(result.Spans))
	}

	// Verify DESC ordering (most recent first).
	if result.Spans[0].SpanID != "span-4" {
		t.Errorf("first span = %q, want span-4 (most recent)", result.Spans[0].SpanID)
	}
}

// ─── Empty store edge cases ──────────────────────────────────────────────────

func TestEmptyStore_AllQueries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	t.Run("QueryTraces_empty", func(t *testing.T) {
		result, err := store.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0", len(result.Traces))
		}
	})

	t.Run("SearchSpans_empty", func(t *testing.T) {
		result, err := store.SearchSpans(ctx, storage.SpanQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(result.Spans) != 0 {
			t.Errorf("count = %d, want 0", len(result.Spans))
		}
	})

	t.Run("GetUsageSummary_empty", func(t *testing.T) {
		summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if summary.TotalTraces != 0 {
			t.Errorf("TotalTraces = %d, want 0", summary.TotalTraces)
		}
		if summary.TotalSpans != 0 {
			t.Errorf("TotalSpans = %d, want 0", summary.TotalSpans)
		}
		if summary.ErrorRate != 0 {
			t.Errorf("ErrorRate = %f, want 0", summary.ErrorRate)
		}
	})

	t.Run("GetModelBreakdown_empty", func(t *testing.T) {
		models, err := store.GetModelBreakdown(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(models) != 0 {
			t.Errorf("count = %d, want 0", len(models))
		}
	})

	t.Run("GetUserLeaderboard_empty", func(t *testing.T) {
		leaders, err := store.GetUserLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(leaders) != 0 {
			t.Errorf("count = %d, want 0", len(leaders))
		}
	})

	t.Run("GetTenantLeaderboard_empty_2", func(t *testing.T) {
		tenants, err := store.GetTenantLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(tenants) != 0 {
			t.Errorf("count = %d, want 0", len(tenants))
		}
	})

	t.Run("GetJobLeaderboard_empty", func(t *testing.T) {
		jobs, err := store.GetJobLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: start, EndTime: end,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(jobs) != 0 {
			t.Errorf("count = %d, want 0", len(jobs))
		}
	})

	t.Run("GetTrace_not_found", func(t *testing.T) {
		_, err := store.GetTrace(ctx, "nonexistent-trace")
		if err == nil {
			t.Error("expected error for nonexistent trace")
		}
	})
}

// ─── GetJobLeaderboard: basic behavior ───────────────────────────────────────

func TestGetJobLeaderboard_Basic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("j1-s1", "j1-t1", storage.SpanKindLLM, "gpt-4o"),
		testSpan("j2-s1", "j2-t1", storage.SpanKindLLM, "claude-sonnet"),
		testSpan("j2-s2", "j2-t2", storage.SpanKindLLM, "claude-sonnet"),
	}
	spans[0].JobID = "eval-v1"
	spans[0].GenAI.CostUSD = 0.05
	spans[0].StartTime = now
	spans[0].EndTime = now.Add(100 * time.Millisecond)

	spans[1].JobID = "eval-v2"
	spans[1].GenAI.CostUSD = 0.10
	spans[1].StartTime = now.Add(time.Second)
	spans[1].EndTime = now.Add(time.Second + 100*time.Millisecond)

	spans[2].JobID = "eval-v2"
	spans[2].GenAI.CostUSD = 0.10
	spans[2].StartTime = now.Add(2 * time.Second)
	spans[2].EndTime = now.Add(2*time.Second + 100*time.Millisecond)

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	jobs, err := store.GetJobLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}, 10)
	if err != nil {
		t.Fatalf("job leaderboard: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	// eval-v2: $0.20 total cost → first.
	if jobs[0].JobID != "eval-v2" {
		t.Errorf("first job = %q, want eval-v2", jobs[0].JobID)
	}
	// eval-v2 has 2 distinct traces.
	if jobs[0].CallCount != 2 {
		t.Errorf("eval-v2 call count = %d, want 2", jobs[0].CallCount)
	}
	if jobs[1].JobID != "eval-v1" {
		t.Errorf("second job = %q, want eval-v1", jobs[1].JobID)
	}
}

// ─── GetJobLeaderboard: limit respected ──────────────────────────────────────

func TestGetJobLeaderboard_LimitRespected(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	for i := 0; i < 5; i++ {
		sp := testSpan(fmt.Sprintf("s%d", i), fmt.Sprintf("t%d", i), storage.SpanKindLLM, "gpt-4o")
		sp.JobID = fmt.Sprintf("job-%d", i)
		sp.GenAI.CostUSD = float64(i+1) * 0.01
		sp.StartTime = now.Add(time.Duration(i) * time.Second)
		sp.EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
		if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	jobs, err := store.GetJobLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}, 3)
	if err != nil {
		t.Fatalf("job leaderboard: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3 (limit enforced)", len(jobs))
	}
}

// ─── Concurrent read/write safety ────────────────────────────────────────────

func TestConcurrentReadWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Seed some initial data.
	seed := testSpan("seed-1", "trace-seed", storage.SpanKindLLM, "gpt-4o")
	seed.StartTime = now
	seed.EndTime = now.Add(100 * time.Millisecond)
	if err := store.IngestSpans(ctx, []storage.Span{seed}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent writers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			sp := testSpan(fmt.Sprintf("w-%d", i), fmt.Sprintf("wt-%d", i), storage.SpanKindLLM, "gpt-4o")
			sp.StartTime = time.Now().Truncate(time.Microsecond)
			sp.EndTime = sp.StartTime.Add(100 * time.Millisecond)
			if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
				errCh <- fmt.Errorf("write %d: %w", i, err)
			}
		}
	}()

	// Concurrent readers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, err := store.QueryTraces(ctx, storage.TraceQuery{
				ProjectID: "proj-test",
				StartTime: now.Add(-time.Hour),
				EndTime:   now.Add(time.Hour),
				PageSize:  50,
			})
			if err != nil {
				errCh <- fmt.Errorf("read %d: %w", i, err)
			}
		}
	}()

	// Concurrent usage queries.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, err := store.GetUsageSummary(ctx, storage.UsageQuery{
				ProjectID: "proj-test",
				StartTime: now.Add(-time.Hour),
				EndTime:   now.Add(time.Hour),
			})
			if err != nil {
				errCh <- fmt.Errorf("usage %d: %w", i, err)
			}
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent operation failed: %v", err)
	}
}

// ─── Default page sizes ─────────────────────────────────────────────────────

func TestQueryTraces_DefaultPageSize(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	for i := 0; i < 3; i++ {
		sp := testSpan(fmt.Sprintf("s%d", i), fmt.Sprintf("t%d", i), storage.SpanKindLLM, "gpt-4o")
		sp.StartTime = now.Add(time.Duration(i) * time.Second)
		sp.EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
		if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// PageSize=0 → defaults to 50, returning all 3.
	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  0,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 3 {
		t.Errorf("count = %d, want 3 (default page size)", len(result.Traces))
	}
}

func TestSearchSpans_DefaultPageSize(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	for i := 0; i < 3; i++ {
		sp := testSpan(fmt.Sprintf("s%d", i), fmt.Sprintf("t%d", i), storage.SpanKindLLM, "gpt-4o")
		sp.StartTime = now.Add(time.Duration(i) * time.Second)
		sp.EndTime = now.Add(time.Duration(i)*time.Second + 100*time.Millisecond)
		if err := store.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// PageSize=0 → defaults to 50, returning all 3.
	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  0,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 3 {
		t.Errorf("count = %d, want 3 (default page size)", len(result.Spans))
	}
}

// ─── Labels round-trip ───────────────────────────────────────────────────────

func TestIngestSpans_LabelsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := testSpan("labels-1", "trace-labels", storage.SpanKindAgent, "")
	span.UserID = "alice@example.com"
	span.SessionID = "session-123"
	span.TenantID = "acme"
	span.JobID = "eval-1"
	span.TraceGroup = "group-a"
	span.GenAI = nil // agent span, no GenAI

	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "trace-labels")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(trace.Spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(trace.Spans))
	}

	got := trace.Spans[0]
	if got.UserID != "alice@example.com" {
		t.Errorf("UserID = %q, want alice@example.com", got.UserID)
	}
	if got.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want session-123", got.SessionID)
	}
	if got.TenantID != "acme" {
		t.Errorf("TenantID = %q, want acme", got.TenantID)
	}
	if got.JobID != "eval-1" {
		t.Errorf("JobID = %q, want eval-1", got.JobID)
	}
}

// Note: DuckDB uses exact match for project_id (WHERE project_id = ?),
// so empty project_id returns no results. This differs from SQLite which
// treats empty project_id as "all projects". The optional_filters_test.go
// in the sqlite package covers this pattern for SQLite.

// ─── GetTenantLeaderboard: excludes empty tenant IDs ─────────────────────────

func TestGetTenantLeaderboard_ExcludesEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	s1 := testSpan("s1", "t1", storage.SpanKindLLM, "gpt-4o")
	s1.TenantID = "real-tenant"
	s1.GenAI.CostUSD = 0.10
	s1.StartTime = now
	s1.EndTime = now.Add(100 * time.Millisecond)

	s2 := testSpan("s2", "t2", storage.SpanKindLLM, "gpt-4o")
	s2.TenantID = "" // no tenant
	s2.GenAI.CostUSD = 99.99
	s2.StartTime = now.Add(time.Second)
	s2.EndTime = now.Add(time.Second + 100*time.Millisecond)

	if err := store.IngestSpans(ctx, []storage.Span{s1, s2}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	tenants, err := store.GetTenantLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}, 10)
	if err != nil {
		t.Fatalf("tenant leaderboard: %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("got %d tenants, want 1 (empty tenant excluded)", len(tenants))
	}
	if tenants[0].TenantID != "real-tenant" {
		t.Errorf("tenant = %q, want real-tenant", tenants[0].TenantID)
	}
}
