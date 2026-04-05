package duckdb

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/candelahq/candela/pkg/storage"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	// Use a unique in-memory database per test.
	// DuckDB treats empty string as default file; use ":memory:" for true in-memory.
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("opening duckdb: %v", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func testSpan(id, traceID string, kind storage.SpanKind, model string) storage.Span {
	now := time.Now().Truncate(time.Microsecond) // DuckDB TIMESTAMP is microsecond precision
	return storage.Span{
		SpanID:       id,
		TraceID:      traceID,
		ParentSpanID: "",
		Name:         "test." + id,
		Kind:         kind,
		Status:       storage.SpanStatusOK,
		StartTime:    now,
		EndTime:      now.Add(100 * time.Millisecond),
		Duration:     100 * time.Millisecond,
		ProjectID:    "proj-test",
		Environment:  "test",
		ServiceName:  "candela-test",
		GenAI: &storage.GenAIAttributes{
			Model:        model,
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.001,
		},
		Attributes: map[string]string{
			"env":     "test",
			"version": "1.0",
		},
	}
}

func TestNew_InMemory(t *testing.T) {
	store := newTestStore(t)
	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestIngestSpans_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := testSpan("span-1", "trace-1", storage.SpanKindLLM, "gpt-4o")

	// Ingest
	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Read back
	trace, err := store.GetTrace(ctx, "trace-1")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}

	if len(trace.Spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(trace.Spans))
	}

	got := trace.Spans[0]
	if got.SpanID != "span-1" {
		t.Errorf("SpanID = %q, want %q", got.SpanID, "span-1")
	}
	if got.TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want %q", got.TraceID, "trace-1")
	}
	if got.Kind != storage.SpanKindLLM {
		t.Errorf("Kind = %d, want %d", got.Kind, storage.SpanKindLLM)
	}
	if got.GenAI == nil {
		t.Fatal("GenAI is nil")
	}
	if got.GenAI.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", got.GenAI.Model, "gpt-4o")
	}
	if got.GenAI.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", got.GenAI.TotalTokens)
	}
}

func TestIngestSpans_Attributes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := testSpan("span-attr", "trace-attr", storage.SpanKindLLM, "gemini-2.0")
	span.Attributes = map[string]string{
		"deployment.region": "us-central1",
		"user.tier":         "premium",
	}

	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "trace-attr")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}

	got := trace.Spans[0]
	if len(got.Attributes) != 2 {
		t.Fatalf("attributes count = %d, want 2", len(got.Attributes))
	}
	if got.Attributes["deployment.region"] != "us-central1" {
		t.Errorf("Attributes[deployment.region] = %q, want %q", got.Attributes["deployment.region"], "us-central1")
	}
	if got.Attributes["user.tier"] != "premium" {
		t.Errorf("Attributes[user.tier] = %q, want %q", got.Attributes["user.tier"], "premium")
	}
}

func TestIngestSpans_NilGenAI(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := storage.Span{
		SpanID:    "span-noai",
		TraceID:   "trace-noai",
		Name:      "http.request",
		Kind:      storage.SpanKindGeneral,
		Status:    storage.SpanStatusOK,
		StartTime: time.Now().Truncate(time.Microsecond),
		EndTime:   time.Now().Add(50 * time.Millisecond).Truncate(time.Microsecond),
		Duration:  50 * time.Millisecond,
		ProjectID: "proj-test",
	}

	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "trace-noai")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Spans[0].GenAI != nil {
		t.Error("GenAI should be nil for non-LLM span")
	}
}

func TestGetTrace_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetTrace(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing trace")
	}
}

func TestQueryTraces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("s1", "trace-a", storage.SpanKindLLM, "gpt-4o"),
		testSpan("s2", "trace-b", storage.SpanKindLLM, "gemini-2.0"),
	}
	// Spread them in time so ordering is deterministic.
	spans[0].StartTime = now.Add(-2 * time.Second)
	spans[0].EndTime = now.Add(-1 * time.Second)
	spans[1].StartTime = now.Add(-1 * time.Second)
	spans[1].EndTime = now

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

	if len(result.Traces) != 2 {
		t.Fatalf("trace count = %d, want 2", len(result.Traces))
	}
	// Most recent first.
	if result.Traces[0].TraceID != "trace-b" {
		t.Errorf("first trace = %q, want trace-b (most recent)", result.Traces[0].TraceID)
	}
}

func TestSearchSpans_FilterByKind(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("llm-1", "trace-mix", storage.SpanKindLLM, "gpt-4o"),
		{
			SpanID: "tool-1", TraceID: "trace-mix", Name: "search",
			Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
		Kind:      storage.SpanKindLLM,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Spans) != 1 {
		t.Fatalf("span count = %d, want 1 (only LLM)", len(result.Spans))
	}
	if result.Spans[0].SpanID != "llm-1" {
		t.Errorf("SpanID = %q, want llm-1", result.Spans[0].SpanID)
	}
}

func TestGetUsageSummary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("s1", "trace-u1", storage.SpanKindLLM, "gpt-4o"),
		testSpan("s2", "trace-u2", storage.SpanKindLLM, "gpt-4o"),
	}
	spans[0].StartTime = now
	spans[0].EndTime = now.Add(100 * time.Millisecond)
	spans[1].StartTime = now
	spans[1].EndTime = now.Add(200 * time.Millisecond)

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	summary, err := store.GetUsageSummary(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	if summary.TotalTraces != 2 {
		t.Errorf("TotalTraces = %d, want 2", summary.TotalTraces)
	}
	if summary.TotalSpans != 2 {
		t.Errorf("TotalSpans = %d, want 2", summary.TotalSpans)
	}
	if summary.TotalLLMCalls != 2 {
		t.Errorf("TotalLLMCalls = %d, want 2", summary.TotalLLMCalls)
	}
	// Each span has 100 input tokens.
	if summary.TotalInputTokens != 200 {
		t.Errorf("TotalInputTokens = %d, want 200", summary.TotalInputTokens)
	}
}

func TestGetModelBreakdown(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		testSpan("s1", "t1", storage.SpanKindLLM, "gpt-4o"),
		testSpan("s2", "t2", storage.SpanKindLLM, "gpt-4o"),
		testSpan("s3", "t3", storage.SpanKindLLM, "gemini-2.0"),
	}
	for i := range spans {
		spans[i].StartTime = now
		spans[i].EndTime = now.Add(100 * time.Millisecond)
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	models, err := store.GetModelBreakdown(ctx, storage.UsageQuery{
		ProjectID: "proj-test",
		StartTime: now.Add(-10 * time.Second),
		EndTime:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("model breakdown: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("model count = %d, want 2", len(models))
	}

	// Both models have same cost per call, but gpt-4o has 2 calls.
	byModel := make(map[string]storage.ModelUsage)
	for _, m := range models {
		byModel[m.Model] = m
	}
	if byModel["gpt-4o"].CallCount != 2 {
		t.Errorf("gpt-4o calls = %d, want 2", byModel["gpt-4o"].CallCount)
	}
	if byModel["gemini-2.0"].CallCount != 1 {
		t.Errorf("gemini-2.0 calls = %d, want 1", byModel["gemini-2.0"].CallCount)
	}
}

func TestIngestSpans_EmptyBatch(t *testing.T) {
	store := newTestStore(t)
	// Should not error on empty input.
	if err := store.IngestSpans(context.Background(), nil); err != nil {
		t.Fatalf("ingest empty: %v", err)
	}
	if err := store.IngestSpans(context.Background(), []storage.Span{}); err != nil {
		t.Fatalf("ingest empty slice: %v", err)
	}
}

func TestBuildTrace_Aggregation(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "root", TraceID: "t1", Name: "agent.run",
			Kind: storage.SpanKindAgent, StartTime: now, EndTime: now.Add(2 * time.Second),
			ProjectID: "p1", Environment: "prod",
		},
		{
			SpanID: "llm-1", TraceID: "t1", ParentSpanID: "root", Name: "llm.chat",
			Kind: storage.SpanKindLLM, StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond),
			ProjectID: "p1",
			GenAI:     &storage.GenAIAttributes{TotalTokens: 1000, CostUSD: 0.01},
		},
		{
			SpanID: "llm-2", TraceID: "t1", ParentSpanID: "root", Name: "llm.chat",
			Kind: storage.SpanKindLLM, StartTime: now.Add(1 * time.Second), EndTime: now.Add(1500 * time.Millisecond),
			ProjectID: "p1",
			GenAI:     &storage.GenAIAttributes{TotalTokens: 2000, CostUSD: 0.02},
		},
	}

	trace := buildTrace("t1", spans)

	if trace.RootSpanName != "agent.run" {
		t.Errorf("RootSpanName = %q, want agent.run", trace.RootSpanName)
	}
	if trace.TotalTokens != 3000 {
		t.Errorf("TotalTokens = %d, want 3000", trace.TotalTokens)
	}
	if trace.TotalCostUSD < 0.029 || trace.TotalCostUSD > 0.031 {
		t.Errorf("TotalCostUSD = %f, want ~0.03", trace.TotalCostUSD)
	}
	if trace.SpanCount != 3 {
		t.Errorf("SpanCount = %d, want 3", trace.SpanCount)
	}
	if trace.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", trace.Duration)
	}
}

func TestIngestSpans_EmptyAttributes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := storage.Span{
		SpanID:     "span-empty-attrs",
		TraceID:    "trace-empty-attrs",
		Name:       "test.empty",
		Kind:       storage.SpanKindLLM,
		Status:     storage.SpanStatusOK,
		StartTime:  time.Now().Truncate(time.Microsecond),
		EndTime:    time.Now().Add(50 * time.Millisecond).Truncate(time.Microsecond),
		Duration:   50 * time.Millisecond,
		ProjectID:  "proj-test",
		Attributes: map[string]string{}, // empty, not nil
	}

	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	trace, err := store.GetTrace(ctx, "trace-empty-attrs")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}

	got := trace.Spans[0]
	// Empty attributes should round-trip as nil or empty map — not cause errors.
	if len(got.Attributes) != 0 {
		t.Errorf("Attributes = %v, want nil or empty", got.Attributes)
	}
}

func TestIngestSpans_DuplicateAllowed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	span := testSpan("dup-1", "trace-dup", storage.SpanKindLLM, "gpt-4o")

	// Ingest same span twice — should not error (Option C: no PK constraint).
	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if err := store.IngestSpans(ctx, []storage.Span{span}); err != nil {
		t.Fatalf("second ingest (duplicate): %v", err)
	}

	// Both rows exist — OLAP convention, dedup at query time.
	trace, err := store.GetTrace(ctx, "trace-dup")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(trace.Spans) != 2 {
		t.Errorf("span count = %d, want 2 (duplicates allowed)", len(trace.Spans))
	}
}

func TestSearchSpans_NameSubstring(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "llm.chat.completion",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
		{
			SpanID: "s2", TraceID: "t2", Name: "tool.search",
			Kind: storage.SpanKindTool, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
		{
			SpanID: "s3", TraceID: "t3", Name: "agent.chat.orchestrator",
			Kind: storage.SpanKindAgent, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Search for "chat" — should match s1 and s3.
	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID:    "proj-test",
		StartTime:    now.Add(-10 * time.Second),
		EndTime:      now.Add(10 * time.Second),
		NameContains: "chat",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Spans) != 2 {
		t.Fatalf("span count = %d, want 2 (matching 'chat')", len(result.Spans))
	}
}

func TestSearchSpans_NameWildcardEscape(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "s1", TraceID: "t1", Name: "tokens_100%_used",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
		{
			SpanID: "s2", TraceID: "t2", Name: "tokens_50_used",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Duration: 50 * time.Millisecond, ProjectID: "proj-test",
		},
	}

	if err := store.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Search for "100%" — should match only s1, not s2.
	// Without ESCAPE, the % would act as wildcard and match both.
	result, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID:    "proj-test",
		StartTime:    now.Add(-10 * time.Second),
		EndTime:      now.Add(10 * time.Second),
		NameContains: "100%",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Spans) != 1 {
		t.Fatalf("span count = %d, want 1 (only exact '100%%' match)", len(result.Spans))
	}
	if result.Spans[0].SpanID != "s1" {
		t.Errorf("SpanID = %q, want s1", result.Spans[0].SpanID)
	}
}
