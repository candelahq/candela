package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("creating test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestIngestAndGetTrace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "span-root", TraceID: "trace-1", Name: "agent.run",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(2 * time.Second),
			Duration: 2 * time.Second, ProjectID: "proj-1",
		},
		{
			SpanID: "span-llm", TraceID: "trace-1", ParentSpanID: "span-root",
			Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(100 * time.Millisecond),
			EndTime:   now.Add(900 * time.Millisecond),
			Duration:  800 * time.Millisecond, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gemini-2.0-flash", Provider: "google",
				InputTokens: 500, OutputTokens: 200, TotalTokens: 700,
				CostUSD: 0.00013,
			},
		},
		{
			SpanID: "span-tool", TraceID: "trace-1", ParentSpanID: "span-root",
			Name: "tool.search", Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now.Add(1 * time.Second),
			EndTime:   now.Add(1500 * time.Millisecond),
			Duration:  500 * time.Millisecond, ProjectID: "proj-1",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	trace, err := s.GetTrace(ctx, "trace-1")
	if err != nil {
		t.Fatalf("get trace failed: %v", err)
	}

	if trace.TraceID != "trace-1" {
		t.Errorf("trace_id = %s, want trace-1", trace.TraceID)
	}
	if trace.SpanCount != 3 {
		t.Errorf("span_count = %d, want 3", trace.SpanCount)
	}
	if trace.RootSpanName != "agent.run" {
		t.Errorf("root_span_name = %s, want agent.run", trace.RootSpanName)
	}
	if trace.TotalTokens != 700 {
		t.Errorf("total_tokens = %d, want 700", trace.TotalTokens)
	}
	if trace.TotalCostUSD < 0.00012 || trace.TotalCostUSD > 0.00014 {
		t.Errorf("total_cost = %f, want ~0.00013", trace.TotalCostUSD)
	}
}

func TestGetTraceNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetTrace(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent trace")
	}
}

func TestQueryTraces(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Ingest 3 traces.
	for i, name := range []string{"trace-a", "trace-b", "trace-c"} {
		offset := time.Duration(i) * time.Minute
		spans := []storage.Span{
			{
				SpanID: "root-" + name, TraceID: name, Name: "root",
				Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
				StartTime: now.Add(offset), EndTime: now.Add(offset + time.Second),
				Duration: time.Second, ProjectID: "proj-1",
			},
			{
				SpanID: "llm-" + name, TraceID: name, ParentSpanID: "root-" + name,
				Name: "llm.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
				StartTime: now.Add(offset + 100*time.Millisecond),
				EndTime:   now.Add(offset + 500*time.Millisecond),
				Duration:  400 * time.Millisecond, ProjectID: "proj-1",
				GenAI: &storage.GenAIAttributes{
					Model: "gpt-4o", Provider: "openai",
					InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
					CostUSD: 0.0005,
				},
			},
		}
		if err := s.IngestSpans(ctx, spans); err != nil {
			t.Fatalf("ingest failed: %v", err)
		}
	}

	result, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(result.Traces) != 3 {
		t.Errorf("got %d traces, want 3", len(result.Traces))
	}

	// Verify each trace has correct aggregates.
	for _, tr := range result.Traces {
		if tr.SpanCount != 2 {
			t.Errorf("trace %s: span_count = %d, want 2", tr.TraceID, tr.SpanCount)
		}
		if tr.LLMCallCount != 1 {
			t.Errorf("trace %s: llm_count = %d, want 1", tr.TraceID, tr.LLMCallCount)
		}
		if tr.TotalTokens != 150 {
			t.Errorf("trace %s: tokens = %d, want 150", tr.TraceID, tr.TotalTokens)
		}
	}
}

func TestGetUsageSummary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500,
				CostUSD: 0.0075,
			},
		},
		{
			SpanID: "s2", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusError,
			StartTime: now.Add(time.Second), EndTime: now.Add(2 * time.Second),
			Duration: time.Second, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 200, OutputTokens: 0, TotalTokens: 200,
				CostUSD: 0.0005,
			},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	summary, err := s.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("usage summary failed: %v", err)
	}

	if summary.TotalTraces != 1 {
		t.Errorf("total_traces = %d, want 1", summary.TotalTraces)
	}
	if summary.TotalSpans != 2 {
		t.Errorf("total_spans = %d, want 2", summary.TotalSpans)
	}
	if summary.TotalLLMCalls != 2 {
		t.Errorf("total_llm_calls = %d, want 2", summary.TotalLLMCalls)
	}
	if summary.TotalInputTokens != 1200 {
		t.Errorf("total_input_tokens = %d, want 1200", summary.TotalInputTokens)
	}
	if summary.ErrorRate < 0.4 || summary.ErrorRate > 0.6 {
		t.Errorf("error_rate = %f, want ~0.5", summary.ErrorRate)
	}
}

func TestGetModelBreakdown(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.01,
			},
		},
		{
			SpanID: "s2", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gemini-2.0-flash", Provider: "google",
				InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.001,
			},
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	models, err := s.GetModelBreakdown(ctx, storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("model breakdown failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}

	// Sorted by cost DESC, so gpt-4o first.
	if models[0].Model != "gpt-4o" {
		t.Errorf("first model = %s, want gpt-4o", models[0].Model)
	}
	if models[1].Model != "gemini-2.0-flash" {
		t.Errorf("second model = %s, want gemini-2.0-flash", models[1].Model)
	}
}

func TestSearchSpans(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "llm.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
			GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai"},
		},
		{
			SpanID: "s2", TraceID: "t1", Name: "tool.search",
			Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(time.Second), Duration: time.Second,
			ProjectID: "proj-1",
		},
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	// Search by kind.
	result, err := s.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-1",
		Kind:      storage.SpanKindLLM,
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(result.Spans) != 1 {
		t.Errorf("got %d spans, want 1 (LLM only)", len(result.Spans))
	}
}
