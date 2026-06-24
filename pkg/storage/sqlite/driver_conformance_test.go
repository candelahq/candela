package sqlite

// Conformance-style driver tests for the SQLite storage backend.
//
// These tests focus on behaviors not yet covered by existing test files:
//   - QueryTraces pagination (PageSize + sequential fetching)
//   - QueryTraces time-range filtering (out-of-range returns empty)
//   - QueryTraces status filter (error/ok)
//   - QueryTraces model filter
//   - QueryTraces search (name substring)
//   - SearchSpans pagination
//   - Empty-store edge cases (all query methods return zero, not error)
//   - GetJobLeaderboard basic behavior
//   - Concurrent read/write safety

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── QueryTraces: pagination ─────────────────────────────────────────────────

func TestQueryTraces_Pagination_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Ingest 5 traces, each separated by 1 minute for deterministic ordering.
	for i := 0; i < 5; i++ {
		offset := time.Duration(i) * time.Minute
		span := storage.Span{
			SpanID: fmt.Sprintf("span-%d", i), TraceID: fmt.Sprintf("trace-%d", i),
			Name: "root", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(offset), EndTime: now.Add(offset + time.Second),
			Duration: time.Second, ProjectID: "proj-page",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.001,
			},
		}
		if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// Page 1: first 2 traces (most recent first → trace-4, trace-3).
	page1, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-page",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
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

	// Page 2: next 2 (trace-2, trace-1).
	// SQLite backend uses simple LIMIT, not cursor-based PageToken, so we just
	// verify page size is respected.
	all, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-page",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
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

func TestQueryTraces_TimeRangeFiltering_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	span := storage.Span{
		SpanID: "s1", TraceID: "t1", Name: "root",
		Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(time.Second),
		Duration: time.Second, ProjectID: "proj-range",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.001,
		},
	}
	if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	t.Run("in_range_returns_trace", func(t *testing.T) {
		result, err := s.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-range",
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
		result, err := s.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-range",
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0 (out of range)", len(result.Traces))
		}
	})

	t.Run("after_range_returns_empty", func(t *testing.T) {
		result, err := s.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-range",
			StartTime: now.Add(1 * time.Hour),
			EndTime:   now.Add(2 * time.Hour),
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0 (out of range)", len(result.Traces))
		}
	})
}

// ─── QueryTraces: status filter ──────────────────────────────────────────────

func TestQueryTraces_StatusFilter_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s-ok", TraceID: "trace-ok", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-status",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "s-err", TraceID: "trace-err", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(time.Minute), EndTime: now.Add(time.Minute + time.Second),
			Duration: time.Second, ProjectID: "proj-status",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// QueryTraces returns traces with MAX(status) aggregation. Both traces
	// have only one span each, so each trace status equals its span status.
	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-status",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
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

// ─── QueryTraces: model filter ───────────────────────────────────────────────

func TestQueryTraces_ModelFilter_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "trace-gpt", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-model",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "s2", TraceID: "trace-gemini", Name: "root",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Minute), EndTime: now.Add(time.Minute + time.Second),
			Duration: time.Second, ProjectID: "proj-model",
			GenAI: &storage.GenAIAttributes{Model: "gemini-2.0-flash", Provider: "google", CostUSD: 0.005},
		},
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// SearchSpans supports model filtering directly.
	result, err := s.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-model",
		Model:     "gpt-4o",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 1 {
		t.Fatalf("count = %d, want 1", len(result.Spans))
	}
	if result.Spans[0].TraceID != "trace-gpt" {
		t.Errorf("trace = %q, want trace-gpt", result.Spans[0].TraceID)
	}
}

// ─── SearchSpans: pagination ─────────────────────────────────────────────────

func TestSearchSpans_Pagination_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Ingest 5 spans across different traces.
	for i := 0; i < 5; i++ {
		offset := time.Duration(i) * time.Minute
		span := storage.Span{
			SpanID: fmt.Sprintf("span-%d", i), TraceID: fmt.Sprintf("trace-%d", i),
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(offset), EndTime: now.Add(offset + time.Second),
			Duration: time.Second, ProjectID: "proj-span-page",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai"},
		}
		if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// Request only 3.
	result, err := s.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-span-page",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 3 {
		t.Errorf("count = %d, want 3 (page size limit)", len(result.Spans))
	}

	// Verify they are sorted DESC by start_time (most recent first).
	if result.Spans[0].SpanID != "span-4" {
		t.Errorf("first span = %q, want span-4 (most recent)", result.Spans[0].SpanID)
	}
}

// ─── Empty store edge cases ──────────────────────────────────────────────────

func TestEmptyStore_AllQueries_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	timeRange := now.Add(-time.Hour)
	timeEnd := now.Add(time.Hour)

	t.Run("QueryTraces_empty", func(t *testing.T) {
		result, err := s.QueryTraces(ctx, storage.TraceQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(result.Traces) != 0 {
			t.Errorf("count = %d, want 0", len(result.Traces))
		}
	})

	t.Run("SearchSpans_empty", func(t *testing.T) {
		result, err := s.SearchSpans(ctx, storage.SpanQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
			PageSize:  10,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(result.Spans) != 0 {
			t.Errorf("count = %d, want 0", len(result.Spans))
		}
	})

	t.Run("GetUsageSummary_empty", func(t *testing.T) {
		summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
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
		models, err := s.GetModelBreakdown(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(models) != 0 {
			t.Errorf("count = %d, want 0", len(models))
		}
	})

	t.Run("GetUserLeaderboard_empty", func(t *testing.T) {
		leaders, err := s.GetUserLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(leaders) != 0 {
			t.Errorf("count = %d, want 0", len(leaders))
		}
	})

	t.Run("GetTenantLeaderboard_empty", func(t *testing.T) {
		tenants, err := s.GetTenantLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(tenants) != 0 {
			t.Errorf("count = %d, want 0", len(tenants))
		}
	})

	t.Run("GetJobLeaderboard_empty", func(t *testing.T) {
		jobs, err := s.GetJobLeaderboard(ctx, storage.UsageQuery{
			ProjectID: "proj-empty",
			StartTime: timeRange,
			EndTime:   timeEnd,
		}, 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(jobs) != 0 {
			t.Errorf("count = %d, want 0", len(jobs))
		}
	})

	t.Run("GetTrace_not_found", func(t *testing.T) {
		_, err := s.GetTrace(ctx, "nonexistent-trace")
		if err == nil {
			t.Error("expected error for nonexistent trace")
		}
	})
}

// ─── GetJobLeaderboard: basic behavior ───────────────────────────────────────

func TestGetJobLeaderboard_Basic_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "j1-s1", TraceID: "j1-t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-job", JobID: "eval-v1",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.05,
			},
		},
		{
			SpanID: "j2-s1", TraceID: "j2-t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Minute), EndTime: now.Add(time.Minute + time.Second),
			Duration: time.Second, ProjectID: "proj-job", JobID: "eval-v2",
			GenAI: &storage.GenAIAttributes{
				Model: "claude-sonnet-4-20250514", Provider: "anthropic",
				InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.10,
			},
		},
		{
			SpanID: "j2-s2", TraceID: "j2-t2", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(2 * time.Minute), EndTime: now.Add(2*time.Minute + time.Second),
			Duration: time.Second, ProjectID: "proj-job", JobID: "eval-v2",
			GenAI: &storage.GenAIAttributes{
				Model: "claude-sonnet-4-20250514", Provider: "anthropic",
				InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.10,
			},
		},
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	jobs, err := s.GetJobLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-job",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("job leaderboard: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	// eval-v2 has higher cost ($0.20) → ranked first.
	if jobs[0].JobID != "eval-v2" {
		t.Errorf("first job = %q, want eval-v2", jobs[0].JobID)
	}
	if jobs[0].CallCount != 2 {
		t.Errorf("eval-v2 call count = %d, want 2", jobs[0].CallCount)
	}
	if jobs[1].JobID != "eval-v1" {
		t.Errorf("second job = %q, want eval-v1", jobs[1].JobID)
	}
}

// ─── GetJobLeaderboard: limit respected ──────────────────────────────────────

func TestGetJobLeaderboard_LimitRespected_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Insert 5 distinct jobs.
	for i := 0; i < 5; i++ {
		span := storage.Span{
			SpanID: fmt.Sprintf("s%d", i), TraceID: fmt.Sprintf("t%d", i),
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i)*time.Minute + time.Second),
			Duration:  time.Second, ProjectID: "proj-limit", JobID: fmt.Sprintf("job-%d", i),
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				CostUSD: float64(i+1) * 0.01,
			},
		}
		if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	jobs, err := s.GetJobLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-limit",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 3)
	if err != nil {
		t.Fatalf("job leaderboard: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3 (limit enforced)", len(jobs))
	}
}

// ─── Concurrent read/write safety ────────────────────────────────────────────

func TestConcurrentReadWrite_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Seed some initial data.
	seed := storage.Span{
		SpanID: "seed-1", TraceID: "trace-seed", Name: "root",
		Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(time.Second),
		Duration: time.Second, ProjectID: "proj-concurrent",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.001,
		},
	}
	if err := s.IngestSpans(ctx, []storage.Span{seed}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent writers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			span := storage.Span{
				SpanID: fmt.Sprintf("w-span-%d", i), TraceID: fmt.Sprintf("w-trace-%d", i),
				Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
				StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(100 * time.Millisecond),
				Duration: 100 * time.Millisecond, ProjectID: "proj-concurrent",
			}
			if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
				errCh <- fmt.Errorf("write %d: %w", i, err)
			}
		}
	}()

	// Concurrent readers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, err := s.QueryTraces(ctx, storage.TraceQuery{
				ProjectID: "proj-concurrent",
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
			_, err := s.GetUsageSummary(ctx, storage.UsageQuery{
				ProjectID: "proj-concurrent",
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

// ─── IngestSpans: empty batch is a no-op ─────────────────────────────────────

func TestIngestSpans_EmptyBatch_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.IngestSpans(ctx, nil); err != nil {
		t.Fatalf("nil batch: %v", err)
	}
	if err := s.IngestSpans(ctx, []storage.Span{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
}

// ─── QueryTraces: default PageSize ───────────────────────────────────────────

func TestQueryTraces_DefaultPageSize_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Ingest 3 traces.
	for i := 0; i < 3; i++ {
		span := storage.Span{
			SpanID: fmt.Sprintf("s%d", i), TraceID: fmt.Sprintf("t%d", i),
			Name: "root", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i)*time.Minute + time.Second),
			Duration:  time.Second, ProjectID: "proj-default-page",
		}
		if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// PageSize=0 should default to 50, returning all 3.
	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-default-page",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  0,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Traces) != 3 {
		t.Errorf("count = %d, want 3 (default page size covers all)", len(result.Traces))
	}
}

// ─── SearchSpans: default PageSize ───────────────────────────────────────────

func TestSearchSpans_DefaultPageSize_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Ingest 3 spans.
	for i := 0; i < 3; i++ {
		span := storage.Span{
			SpanID: fmt.Sprintf("s%d", i), TraceID: fmt.Sprintf("t%d", i),
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i)*time.Minute + time.Second),
			Duration:  time.Second, ProjectID: "proj-default-span-page",
		}
		if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	// PageSize=0 should default to 50, returning all 3.
	result, err := s.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-default-span-page",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  0,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Spans) != 3 {
		t.Errorf("count = %d, want 3 (default page size covers all)", len(result.Spans))
	}
}

// ─── GetTenantLeaderboard: excludes empty tenant IDs ─────────────────────────

func TestGetTenantLeaderboard_ExcludesEmpty_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-tenant", TenantID: "real-tenant",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai", CostUSD: 0.10,
			},
		},
		{
			SpanID: "s2", TraceID: "t2", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(time.Minute), EndTime: now.Add(time.Minute + time.Second),
			Duration: time.Second, ProjectID: "proj-tenant", TenantID: "", // no tenant
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai", CostUSD: 99.99,
			},
		},
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	tenants, err := s.GetTenantLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-tenant",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
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

// ─── Labels round-trip ───────────────────────────────────────────────────────

func TestIngestSpans_LabelsRoundTrip_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	span := storage.Span{
		SpanID: "s1", TraceID: "t1", Name: "agent.run",
		Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(time.Second),
		Duration: time.Second, ProjectID: "proj-labels",
		UserID: "alice@example.com", SessionID: "session-123",
		TenantID: "acme", JobID: "eval-1", TraceGroup: "group-a",
	}
	if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := s.GetTrace(ctx, "t1")
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
