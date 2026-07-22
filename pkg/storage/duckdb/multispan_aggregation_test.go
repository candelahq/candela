package duckdb

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// TestQueryTraces_MultiSpanAggregation_ModelFilter is a regression test for
// the bug fixed in PR #753. When filtering by model, the query must still
// return ALL spans of matching traces (not just the spans that match the
// model filter). This ensures span_count, total_tokens, duration, and
// root_span_name are computed from the full trace, not the filtered subset.
func TestQueryTraces_MultiSpanAggregation_ModelFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create a multi-span trace:
	//   root (agent) -> llm (gpt-4o) + tool (web_search) + llm2 (gpt-4o)
	root := storage.Span{
		SpanID: "root-1", TraceID: "trace-multi", Name: "agent.run",
		Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(3 * time.Second),
		Duration: 3 * time.Second, ProjectID: "proj-test", Environment: "test",
	}
	llm1 := storage.Span{
		SpanID: "llm-1", TraceID: "trace-multi", ParentSpanID: "root-1",
		Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(1 * time.Second),
		Duration: 900 * time.Millisecond, ProjectID: "proj-test", Environment: "test",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.001,
		},
	}
	tool := storage.Span{
		SpanID: "tool-1", TraceID: "trace-multi", ParentSpanID: "root-1",
		Name: "tool.web_search", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
		StartTime: now.Add(1 * time.Second), EndTime: now.Add(2 * time.Second),
		Duration: 1 * time.Second, ProjectID: "proj-test", Environment: "test",
	}
	llm2 := storage.Span{
		SpanID: "llm-2", TraceID: "trace-multi", ParentSpanID: "root-1",
		Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second),
		Duration: 1 * time.Second, ProjectID: "proj-test", Environment: "test",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.002,
		},
	}

	// Also create a non-matching trace (different model) to verify filtering.
	other := testSpan("other-1", "trace-other", storage.SpanKindLLM, "gemini-2.0")
	other.StartTime = now.Add(-5 * time.Second)
	other.EndTime = now.Add(-4 * time.Second)

	if err := store.IngestSpans(ctx, []storage.Span{root, llm1, tool, llm2, other}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		Model:     "gpt-4o",
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	// Should return exactly 1 trace (the multi-span one).
	if len(result.Traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(result.Traces))
	}

	tr := result.Traces[0]
	if tr.TraceID != "trace-multi" {
		t.Errorf("trace_id = %q, want trace-multi", tr.TraceID)
	}

	// Critical: span_count must be 4 (all spans), not 2 (only gpt-4o spans).
	if tr.SpanCount != 4 {
		t.Errorf("span_count = %d, want 4 (all spans in trace, not just filtered)", tr.SpanCount)
	}

	// total_tokens should be 450 (150 + 300), from both LLM spans.
	if tr.TotalTokens != 450 {
		t.Errorf("total_tokens = %d, want 450", tr.TotalTokens)
	}
}

// TestQueryTraces_MultiSpanAggregation_ProviderFilter verifies that provider
// filtering preserves full trace aggregation.
func TestQueryTraces_MultiSpanAggregation_ProviderFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Multi-span trace: root -> anthropic LLM + openai LLM
	root := storage.Span{
		SpanID: "root-1", TraceID: "trace-mixed", Name: "agent.run",
		Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(2 * time.Second),
		Duration: 2 * time.Second, ProjectID: "proj-test", Environment: "test",
	}
	llmAnthropic := storage.Span{
		SpanID: "llm-ant", TraceID: "trace-mixed", ParentSpanID: "root-1",
		Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(1 * time.Second),
		Duration: 900 * time.Millisecond, ProjectID: "proj-test", Environment: "test",
		GenAI: &storage.GenAIAttributes{
			Model: "claude-3.5", Provider: "anthropic",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.003,
		},
	}
	llmOpenAI := storage.Span{
		SpanID: "llm-oai", TraceID: "trace-mixed", ParentSpanID: "root-1",
		Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now.Add(1 * time.Second), EndTime: now.Add(2 * time.Second),
		Duration: 1 * time.Second, ProjectID: "proj-test", Environment: "test",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 150, OutputTokens: 75, TotalTokens: 225, CostUSD: 0.002,
		},
	}

	// Non-matching trace (only google provider).
	other := testSpan("other-1", "trace-google", storage.SpanKindLLM, "gemini-2.0")
	other.GenAI.Provider = "google"
	other.StartTime = now.Add(-5 * time.Second)
	other.EndTime = now.Add(-4 * time.Second)

	if err := store.IngestSpans(ctx, []storage.Span{root, llmAnthropic, llmOpenAI, other}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		Provider:  "anthropic",
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(result.Traces))
	}

	tr := result.Traces[0]
	// All 3 spans must be counted, not just the anthropic one.
	if tr.SpanCount != 3 {
		t.Errorf("span_count = %d, want 3", tr.SpanCount)
	}

	// Total tokens from both LLM spans.
	if tr.TotalTokens != 525 {
		t.Errorf("total_tokens = %d, want 525", tr.TotalTokens)
	}
}

// TestQueryTraces_MultiSpanAggregation_SearchFilter verifies that search
// filtering preserves full trace aggregation.
func TestQueryTraces_MultiSpanAggregation_SearchFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Multi-span trace where only one span name matches the search term.
	root := storage.Span{
		SpanID: "root-1", TraceID: "trace-search", Name: "agent.run",
		Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
		StartTime: now, EndTime: now.Add(2 * time.Second),
		Duration: 2 * time.Second, ProjectID: "proj-test", Environment: "test",
	}
	llm := storage.Span{
		SpanID: "llm-1", TraceID: "trace-search", ParentSpanID: "root-1",
		Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
		StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(1 * time.Second),
		Duration: 900 * time.Millisecond, ProjectID: "proj-test", Environment: "test",
		GenAI: &storage.GenAIAttributes{
			Model: "gpt-4o", Provider: "openai",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.001,
		},
	}
	tool := storage.Span{
		SpanID: "tool-1", TraceID: "trace-search", ParentSpanID: "root-1",
		Name: "tool.weather_lookup", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
		StartTime: now.Add(1 * time.Second), EndTime: now.Add(2 * time.Second),
		Duration: 1 * time.Second, ProjectID: "proj-test", Environment: "test",
	}

	if err := store.IngestSpans(ctx, []storage.Span{root, llm, tool}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Search for "weather" — only the tool span name matches.
	result, err := store.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		PageSize:  10,
		Search:    "weather",
	})
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(result.Traces))
	}

	tr := result.Traces[0]
	// All 3 spans must be counted even though only one name matched "weather".
	if tr.SpanCount != 3 {
		t.Errorf("span_count = %d, want 3", tr.SpanCount)
	}
}
