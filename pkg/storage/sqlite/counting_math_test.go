package sqlite

// Tests for the corrected counting math (trace-level counts, root-span
// latency, trace-level error rate). These tests verify the specific
// behaviors that were fixed — each test would have FAILED before the fix.

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── CallCount: COUNT(DISTINCT trace_id) vs COUNT(*) ─────────────────────────

// TestUserLeaderboard_CallCount_CountsTraces verifies that CallCount counts
// distinct traces, not individual spans.
func TestUserLeaderboard_CallCount_CountsTraces(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// One trace with 3 spans
	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-agent", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-1", UserID: "alice@example.com",
		},
		{
			SpanID: "llm-1", TraceID: "trace-agent", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(800 * time.Millisecond),
			Duration: 700 * time.Millisecond, ProjectID: "proj-1", UserID: "alice@example.com",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 500, OutputTokens: 200, TotalTokens: 700, CostUSD: 0.005,
			},
		},
		{
			SpanID: "tool-1", TraceID: "trace-agent", ParentSpanID: "root",
			Name: "tool.search", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-1", UserID: "alice@example.com",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	leaders, err := s.GetUserLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}

	if len(leaders) != 1 {
		t.Fatalf("got %d leaders, want 1", len(leaders))
	}
	if leaders[0].CallCount != 1 {
		t.Errorf("CallCount = %d, want 1 (1 trace, not 3 spans)", leaders[0].CallCount)
	}
}

// TestJobLeaderboard_CallCount_CountsTraces verifies the job leaderboard
// counts distinct traces per job.
func TestJobLeaderboard_CallCount_CountsTraces(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		// 1 trace with 3 spans, all in job "eval-run-1"
		{
			SpanID: "root", TraceID: "j-trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1", JobID: "eval-run-1",
		},
		{
			SpanID: "llm-1", TraceID: "j-trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-1", JobID: "eval-run-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "llm-2", TraceID: "j-trace-1", ParentSpanID: "root",
			Name: "llm.retry", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(600 * time.Millisecond), EndTime: now.Add(900 * time.Millisecond),
			Duration: 300 * time.Millisecond, ProjectID: "proj-1", JobID: "eval-run-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.02},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	jobs, err := s.GetJobLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("job leaderboard: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].CallCount != 1 {
		t.Errorf("CallCount = %d, want 1 (1 trace, not 3 spans)", jobs[0].CallCount)
	}
}

// ─── Error Rate: trace-level vs span-level ───────────────────────────────────

// TestGetUsageSummary_ErrorRate_TraceLevelNotSpanLevel verifies error rate
// counts distinct traces with errors, not individual errored spans.
func TestGetUsageSummary_ErrorRate_TraceLevelNotSpanLevel(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Trace 1: root OK, child ERROR → trace has error
		{
			SpanID: "t1-root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
		},
		{
			SpanID: "t1-llm", TraceID: "trace-1", ParentSpanID: "t1-root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-1",
		},
		// Trace 2: all OK
		{
			SpanID: "t2-root", TraceID: "trace-2", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second),
			Duration: time.Second, ProjectID: "proj-1",
		},
		{
			SpanID: "t2-llm", TraceID: "trace-2", ParentSpanID: "t2-root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(2100 * time.Millisecond), EndTime: now.Add(2500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-1",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// 2 traces, 1 errored → 0.5
	if summary.ErrorRate < 0.45 || summary.ErrorRate > 0.55 {
		t.Errorf("ErrorRate = %f, want ~0.5 (1 errored trace / 2 traces)", summary.ErrorRate)
	}
}

// TestGetUsageSummary_ErrorRate_MultipleErrorsInSameTrace verifies that
// multiple errored spans in one trace count as ONE errored trace.
func TestGetUsageSummary_ErrorRate_MultipleErrorsInSameTrace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
		},
		{
			SpanID: "llm-1", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(300 * time.Millisecond),
			Duration: 200 * time.Millisecond, ProjectID: "proj-1",
		},
		{
			SpanID: "llm-2", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.retry", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(400 * time.Millisecond), EndTime: now.Add(600 * time.Millisecond),
			Duration: 200 * time.Millisecond, ProjectID: "proj-1",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// 1 trace, it has errors → 1.0
	if summary.ErrorRate < 0.95 || summary.ErrorRate > 1.05 {
		t.Errorf("ErrorRate = %f, want ~1.0 (1 errored trace / 1 trace)", summary.ErrorRate)
	}
}

// ─── Average Latency: root spans only ────────────────────────────────────────

// TestGetUsageSummary_AvgLatency_RootSpansOnly verifies that average latency
// only considers root spans, not fast internal spans.
func TestGetUsageSummary_AvgLatency_RootSpansOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root span: 2 seconds
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-1",
		},
		// Child LLM: 500ms
		{
			SpanID: "llm", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(600 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		// Child tool: 10ms (very fast)
		{
			SpanID: "tool", TraceID: "trace-1", ParentSpanID: "root",
			Name: "tool.lookup", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(700 * time.Millisecond), EndTime: now.Add(710 * time.Millisecond),
			Duration: 10 * time.Millisecond, ProjectID: "proj-1",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// Root span is 2000ms.
	// Before fix: AVG(2000, 500, 10) ≈ 837ms
	// After fix: AVG(2000) = 2000ms
	if summary.AvgLatencyMs < 1900 || summary.AvgLatencyMs > 2100 {
		t.Errorf("AvgLatencyMs = %.1f, want ~2000 (root span only)", summary.AvgLatencyMs)
	}
}

// TestUserLeaderboard_AvgLatency_RootSpansOnly verifies leaderboard avg
// latency uses root spans only.
func TestUserLeaderboard_AvgLatency_RootSpansOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root: 3 seconds
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(3 * time.Second),
			Duration: 3 * time.Second, ProjectID: "proj-1", UserID: "alice",
		},
		// Child: 5ms
		{
			SpanID: "child", TraceID: "trace-1", ParentSpanID: "root",
			Name: "tool.fast", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(105 * time.Millisecond),
			Duration: 5 * time.Millisecond, ProjectID: "proj-1", UserID: "alice",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	leaders, err := s.GetUserLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}

	if len(leaders) != 1 {
		t.Fatalf("got %d leaders, want 1", len(leaders))
	}

	// Before fix: AVG(3000ms, 5ms) ≈ 1502ms
	// After fix: AVG(3000ms) = 3000ms
	if leaders[0].AvgLatencyMs < 2900 || leaders[0].AvgLatencyMs > 3100 {
		t.Errorf("AvgLatencyMs = %.1f, want ~3000 (root span only)", leaders[0].AvgLatencyMs)
	}
}

// TestGetUsageSummary_AvgLatency_NoRootSpans_ReturnsZero tests the edge case
// where all spans have a parent (W3C trace propagation). Should not crash.
func TestGetUsageSummary_AvgLatency_NoRootSpans_ReturnsZero(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "child", TraceID: "trace-1", ParentSpanID: "external-parent",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	if summary.AvgLatencyMs != 0 {
		t.Errorf("AvgLatencyMs = %.1f, want 0 (no root spans)", summary.AvgLatencyMs)
	}
}

// ─── Primary Model/Provider: cost-based attribution ──────────────────────────

// TestQueryTraces_PrimaryProvider_MatchesModel verifies that the provider
// always corresponds to the primary model, not the last-seen provider.
func TestQueryTraces_PrimaryProvider_MatchesModel(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-provider", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-1",
		},
		// Anthropic model — expensive, should be primary
		{
			SpanID: "llm-1", TraceID: "trace-provider", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "claude-4-sonnet", Provider: "anthropic", CostUSD: 1.00},
		},
		// OpenAI model — cheaper, ingested last
		{
			SpanID: "llm-2", TraceID: "trace-provider", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o-mini", Provider: "openai", CostUSD: 0.001},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	tr := result.Traces[0]

	// Before fix: MAX("openai","anthropic") = "openai" (alphabetical, WRONG)
	// After fix: claude-4-sonnet ($1.00) > gpt-4o-mini ($0.001) → anthropic
	if tr.PrimaryModel != "claude-4-sonnet" {
		t.Errorf("PrimaryModel = %q, want claude-4-sonnet", tr.PrimaryModel)
	}
	if tr.PrimaryProvider != "anthropic" {
		t.Errorf("PrimaryProvider = %q, want anthropic (matches primary model)", tr.PrimaryProvider)
	}
}

// ─── Trace Status: error detection ───────────────────────────────────────────

// TestQueryTraces_Status_DetectsChildErrors verifies that a trace is marked
// as errored if ANY child span has an error status.
func TestQueryTraces_Status_DetectsChildErrors(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-err", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-1",
		},
		{
			SpanID: "llm-fail", TraceID: "trace-err", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(200 * time.Millisecond),
			Duration: 100 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai"},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	if result.Traces[0].Status != storage.SpanStatusError {
		t.Errorf("Status = %d, want %d (error, because child span errored)",
			result.Traces[0].Status, storage.SpanStatusError)
	}
}
