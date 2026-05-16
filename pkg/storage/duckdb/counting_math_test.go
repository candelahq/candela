package duckdb

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
// distinct traces, not individual spans. An agent trace with 5 spans should
// report CallCount=1, not CallCount=5.
func TestUserLeaderboard_CallCount_CountsTraces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// One trace with 3 spans: root → llm → tool
	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-agent", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-test", UserID: "alice@example.com",
		},
		{
			SpanID: "llm-1", TraceID: "trace-agent", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(800 * time.Millisecond),
			Duration: 700 * time.Millisecond, ProjectID: "proj-test", UserID: "alice@example.com",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 500, OutputTokens: 200, TotalTokens: 700, CostUSD: 0.005,
			},
		},
		{
			SpanID: "tool-1", TraceID: "trace-agent", ParentSpanID: "root",
			Name: "tool.search", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test", UserID: "alice@example.com",
		},
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

	if len(leaders) != 1 {
		t.Fatalf("got %d leaders, want 1", len(leaders))
	}

	// BUG FIX: Before fix, this was 3 (one per span). Now it's 1 (one trace).
	if leaders[0].CallCount != 1 {
		t.Errorf("CallCount = %d, want 1 (1 trace, not 3 spans)", leaders[0].CallCount)
	}
}

// TestTenantLeaderboard_CallCount_MultipleTracesMultipleSpans tests that
// the tenant leaderboard correctly counts traces across multiple tenants,
// each with multi-span traces.
func TestTenantLeaderboard_CallCount_MultipleTracesMultipleSpans(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Tenant A: 2 traces, each with 2 spans = 4 total spans, but 2 distinct traces.
		{
			SpanID: "a-root-1", TraceID: "a-trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-test", TenantID: "tenant-a",
		},
		{
			SpanID: "a-llm-1", TraceID: "a-trace-1", ParentSpanID: "a-root-1",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test", TenantID: "tenant-a",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "a-root-2", TraceID: "a-trace-2", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second),
			Duration: time.Second, ProjectID: "proj-test", TenantID: "tenant-a",
		},
		{
			SpanID: "a-llm-2", TraceID: "a-trace-2", ParentSpanID: "a-root-2",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(2100 * time.Millisecond), EndTime: now.Add(2500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test", TenantID: "tenant-a",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		// Tenant B: 1 trace with 1 span.
		{
			SpanID: "b-root", TraceID: "b-trace-1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-test", TenantID: "tenant-b",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.05},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	leaderboard, err := store.GetTenantLeaderboard(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	}, 10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}

	if len(leaderboard) != 2 {
		t.Fatalf("got %d tenants, want 2", len(leaderboard))
	}

	// Tenant B has higher cost ($0.05) so comes first.
	if leaderboard[0].TenantID != "tenant-b" {
		t.Errorf("first tenant = %q, want tenant-b", leaderboard[0].TenantID)
	}
	if leaderboard[0].CallCount != 1 {
		t.Errorf("tenant-b CallCount = %d, want 1", leaderboard[0].CallCount)
	}

	// Tenant A has 4 spans across 2 traces → CallCount should be 2.
	if leaderboard[1].TenantID != "tenant-a" {
		t.Errorf("second tenant = %q, want tenant-a", leaderboard[1].TenantID)
	}
	if leaderboard[1].CallCount != 2 {
		t.Errorf("tenant-a CallCount = %d, want 2 (2 traces, not 4 spans)", leaderboard[1].CallCount)
	}
}

// TestJobLeaderboard_CallCount_CountsTraces verifies the job leaderboard
// counts distinct traces per job.
func TestJobLeaderboard_CallCount_CountsTraces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Job "eval-run-1": 1 trace with 3 spans.
		{
			SpanID: "root", TraceID: "j-trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-test", JobID: "eval-run-1",
		},
		{
			SpanID: "llm-1", TraceID: "j-trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test", JobID: "eval-run-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		{
			SpanID: "llm-2", TraceID: "j-trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(600 * time.Millisecond), EndTime: now.Add(900 * time.Millisecond),
			Duration: 300 * time.Millisecond, ProjectID: "proj-test", JobID: "eval-run-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.02},
		},
	}

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

	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].CallCount != 1 {
		t.Errorf("CallCount = %d, want 1 (1 trace, not 3 spans)", jobs[0].CallCount)
	}
}

// ─── Error Rate: trace-level vs span-level ───────────────────────────────────

// TestGetUsageSummary_ErrorRate_TraceLevelNotSpanLevel verifies that error
// rate counts distinct traces with errors, not individual errored spans.
func TestGetUsageSummary_ErrorRate_TraceLevelNotSpanLevel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Trace 1: root OK, child ERROR → trace is errored (1 errored trace)
		{
			SpanID: "t1-root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-test",
		},
		{
			SpanID: "t1-llm", TraceID: "trace-1", ParentSpanID: "t1-root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		// Trace 2: all OK → trace is NOT errored
		{
			SpanID: "t2-root", TraceID: "trace-2", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second),
			Duration: time.Second, ProjectID: "proj-test",
		},
		{
			SpanID: "t2-llm", TraceID: "trace-2", ParentSpanID: "t2-root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(2100 * time.Millisecond), EndTime: now.Add(2500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// 2 traces total, 1 has an error → error rate = 0.5
	// Before fix: 4 spans total, 1 errored span → error rate = 0.25
	if summary.ErrorRate < 0.45 || summary.ErrorRate > 0.55 {
		t.Errorf("ErrorRate = %f, want ~0.5 (1 errored trace / 2 total traces)", summary.ErrorRate)
	}
}

// TestGetUsageSummary_ErrorRate_MultipleErrorsInSameTrace verifies that
// multiple errored spans within the same trace count as ONE errored trace.
func TestGetUsageSummary_ErrorRate_MultipleErrorsInSameTrace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Trace 1: 3 spans, 2 errored → still 1 errored TRACE
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-test",
		},
		{
			SpanID: "llm-1", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(300 * time.Millisecond),
			Duration: 200 * time.Millisecond, ProjectID: "proj-test",
		},
		{
			SpanID: "llm-2", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.chat.retry", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(400 * time.Millisecond), EndTime: now.Add(600 * time.Millisecond),
			Duration: 200 * time.Millisecond, ProjectID: "proj-test",
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// 1 trace, it has errors → error rate = 1.0
	// Before fix: 3 spans, 2 errored → error rate = 0.67
	if summary.ErrorRate < 0.95 || summary.ErrorRate > 1.05 {
		t.Errorf("ErrorRate = %f, want ~1.0 (1 errored trace / 1 total trace)", summary.ErrorRate)
	}
}

// TestGetUsageSummary_ErrorRate_ZeroWhenNoSpans verifies no divide-by-zero.
func TestGetUsageSummary_ErrorRate_ZeroWhenNoSpans(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	if summary.ErrorRate != 0 {
		t.Errorf("ErrorRate = %f, want 0.0 (no data)", summary.ErrorRate)
	}
}

// ─── Average Latency: root spans only ────────────────────────────────────────

// TestGetUsageSummary_AvgLatency_RootSpansOnly verifies that average latency
// only considers root spans (parent_span_id = ”), not fast internal spans.
func TestGetUsageSummary_AvgLatency_RootSpansOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root span: 2 seconds (this IS the request latency)
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-test",
		},
		// Child LLM span: 500ms
		{
			SpanID: "llm", TraceID: "trace-1", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(600 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
		// Child tool span: 10ms (very fast internal tool)
		{
			SpanID: "tool", TraceID: "trace-1", ParentSpanID: "root",
			Name: "tool.lookup", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(700 * time.Millisecond), EndTime: now.Add(710 * time.Millisecond),
			Duration: 10 * time.Millisecond, ProjectID: "proj-test",
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// Root span is 2000ms.
	// Before fix: AVG(2000ms, 500ms, 10ms) = ~837ms — wildly wrong!
	// After fix: AVG(2000ms) = 2000ms — accurate request latency.
	if summary.AvgLatencyMs < 1900 || summary.AvgLatencyMs > 2100 {
		t.Errorf("AvgLatencyMs = %.1f, want ~2000 (root span only, not averaged with child spans)", summary.AvgLatencyMs)
	}
}

// TestUserLeaderboard_AvgLatency_RootSpansOnly verifies the leaderboard
// computes average latency from root spans only.
func TestUserLeaderboard_AvgLatency_RootSpansOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root span: 3 seconds
		{
			SpanID: "root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(3 * time.Second),
			Duration: 3 * time.Second, ProjectID: "proj-test",
			UserID: "alice@example.com",
		},
		// Child span: 5ms (would drag the average way down if included)
		{
			SpanID: "child", TraceID: "trace-1", ParentSpanID: "root",
			Name: "tool.fast", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(105 * time.Millisecond),
			Duration: 5 * time.Millisecond, ProjectID: "proj-test",
			UserID: "alice@example.com",
		},
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

	if len(leaders) != 1 {
		t.Fatalf("got %d leaders, want 1", len(leaders))
	}

	// Before fix: AVG(3000ms, 5ms) = ~1502ms
	// After fix: AVG(3000ms) = 3000ms
	if leaders[0].AvgLatencyMs < 2900 || leaders[0].AvgLatencyMs > 3100 {
		t.Errorf("AvgLatencyMs = %.1f, want ~3000 (root span only)", leaders[0].AvgLatencyMs)
	}
}

// TestGetUsageSummary_AvgLatency_NoRootSpans_FallsBackToZero tests the edge
// case where all spans have a parent (e.g. W3C trace context propagation).
// AVG(CASE WHEN parent_span_id=” ...) returns NULL → COALESCE to 0.
func TestGetUsageSummary_AvgLatency_NoRootSpans_FallsBackToZero(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "child-1", TraceID: "trace-1", ParentSpanID: "external-parent",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.01},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}

	// No root spans → latency should be 0 (COALESCE NULL → 0), not crash.
	if summary.AvgLatencyMs != 0 {
		t.Errorf("AvgLatencyMs = %.1f, want 0 (no root spans)", summary.AvgLatencyMs)
	}
}

// ─── Primary Model/Provider: cost-based attribution ──────────────────────────

// TestQueryTraces_PrimaryModel_ByHighestCost verifies that the primary model
// is the one with the highest total cost, not alphabetically last (MAX).
func TestQueryTraces_PrimaryModel_ByHighestCost(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root span
		{
			SpanID: "root", TraceID: "trace-mixed", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(3 * time.Second),
			Duration: 3 * time.Second, ProjectID: "proj-test",
		},
		// gpt-4o: called 1x with $0.50 cost
		{
			SpanID: "llm-gpt", TraceID: "trace-mixed", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", CostUSD: 0.50},
		},
		// claude-4-sonnet: called 2x with $0.01 each = $0.02 total
		{
			SpanID: "llm-claude-1", TraceID: "trace-mixed", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "claude-4-sonnet", Provider: "anthropic", CostUSD: 0.01},
		},
		{
			SpanID: "llm-claude-2", TraceID: "trace-mixed", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(2 * time.Second), EndTime: now.Add(2500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "claude-4-sonnet", Provider: "anthropic", CostUSD: 0.01},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Fatalf("got %d traces, want 1", len(result.Traces))
	}

	tr := result.Traces[0]

	// Before fix: MAX("gpt-4o", "claude-4-sonnet") = "gpt-4o" (alphabetical MAX).
	// This happened to be correct by coincidence, but is wrong semantically.
	// After fix: gpt-4o has $0.50 cost vs claude-4-sonnet $0.02 → gpt-4o is primary.
	if tr.PrimaryModel != "gpt-4o" {
		t.Errorf("PrimaryModel = %q, want gpt-4o (highest cost model)", tr.PrimaryModel)
	}
	if tr.PrimaryProvider != "openai" {
		t.Errorf("PrimaryProvider = %q, want openai (provider of highest cost model)", tr.PrimaryProvider)
	}
}

// TestQueryTraces_PrimaryProvider_MatchesModel verifies that the provider
// always corresponds to the primary model, not the last-seen provider.
func TestQueryTraces_PrimaryProvider_MatchesModel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "trace-provider", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-test",
		},
		// Anthropic model — expensive, should be primary
		{
			SpanID: "llm-1", TraceID: "trace-provider", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			Duration: 400 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "claude-4-sonnet", Provider: "anthropic", CostUSD: 1.00},
		},
		// OpenAI model — cheaper, ingested last (old bug: provider = "openai")
		{
			SpanID: "llm-2", TraceID: "trace-provider", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			Duration: 500 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o-mini", Provider: "openai", CostUSD: 0.001},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	tr := result.Traces[0]

	// Before fix (BigQuery): primaryProvider = "openai" (last-seen, WRONG)
	// Before fix (SQLite/DuckDB): MAX("openai","anthropic") = "openai" (alphabetical, WRONG)
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
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		// Root: OK (status=1)
		{
			SpanID: "root", TraceID: "trace-err", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second),
			Duration: time.Second, ProjectID: "proj-test",
		},
		// Child: ERROR (status=2)
		{
			SpanID: "llm-fail", TraceID: "trace-err", ParentSpanID: "root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(200 * time.Millisecond),
			Duration: 100 * time.Millisecond, ProjectID: "proj-test",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai"},
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Fatalf("got %d traces, want 1", len(result.Traces))
	}

	// Trace should have error status because child span errored.
	if result.Traces[0].Status != storage.SpanStatusError {
		t.Errorf("Status = %d, want %d (error, because child span errored)",
			result.Traces[0].Status, storage.SpanStatusError)
	}
}
