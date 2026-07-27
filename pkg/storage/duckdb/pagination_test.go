package duckdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func TestQueryTraces_Pagination_Cursor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Insert 120 traces with distinct timestamps
	var spans []storage.Span
	for i := 0; i < 120; i++ {
		traceID := fmt.Sprintf("trace-%03d", i)
		ts := now.Add(time.Duration(i) * time.Minute)
		spans = append(spans, storage.Span{
			SpanID:    "span-" + traceID,
			TraceID:   traceID,
			Name:      "root",
			Kind:      storage.SpanKindLLM,
			Status:    storage.SpanStatusOK,
			StartTime: ts,
			EndTime:   ts.Add(time.Second),
			Duration:  time.Second,
			ProjectID: "proj-1",
			GenAI:     &storage.GenAIAttributes{Model: "gpt-4o"},
		})
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	// Page 1
	res1, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(240 * time.Minute),
		PageSize:  50,
	})
	if err != nil {
		t.Fatalf("query page 1 failed: %v", err)
	}
	if len(res1.Traces) != 50 {
		t.Fatalf("page 1 traces = %d, want 50", len(res1.Traces))
	}
	if res1.NextPageToken == "" {
		t.Fatal("page 1 expected non-empty NextPageToken")
	}
	// Default sort is DESC, so first trace should be trace-119
	if res1.Traces[0].TraceID != "trace-119" {
		t.Errorf("page 1 first trace = %s, want trace-119", res1.Traces[0].TraceID)
	}

	// Page 2
	res2, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(240 * time.Minute),
		PageSize:  50,
		PageToken: res1.NextPageToken,
	})
	if err != nil {
		t.Fatalf("query page 2 failed: %v", err)
	}
	if len(res2.Traces) != 50 {
		t.Fatalf("page 2 traces = %d, want 50", len(res2.Traces))
	}
	if res2.NextPageToken == "" {
		t.Fatal("page 2 expected non-empty NextPageToken")
	}

	// Page 3
	res3, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(240 * time.Minute),
		PageSize:  50,
		PageToken: res2.NextPageToken,
	})
	if err != nil {
		t.Fatalf("query page 3 failed: %v", err)
	}
	if len(res3.Traces) != 20 {
		t.Fatalf("page 3 traces = %d, want 20", len(res3.Traces))
	}
	if res3.NextPageToken != "" {
		t.Errorf("page 3 expected empty NextPageToken, got %q", res3.NextPageToken)
	}
}

func TestQueryTraces_Pagination_CostOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	var spans []storage.Span
	for i := 0; i < 100; i++ {
		traceID := fmt.Sprintf("trace-%03d", i)
		ts := now.Add(time.Duration(i) * time.Minute)
		spans = append(spans, storage.Span{
			SpanID:    "span-" + traceID,
			TraceID:   traceID,
			Name:      "root",
			Kind:      storage.SpanKindLLM,
			Status:    storage.SpanStatusOK,
			StartTime: ts,
			EndTime:   ts.Add(time.Second),
			Duration:  time.Second,
			ProjectID: "proj-cost",
			GenAI:     &storage.GenAIAttributes{Model: "gpt-4o", CostUSD: float64(i) * 0.1},
		})
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	res1, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:  "proj-cost",
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(240 * time.Minute),
		PageSize:   60,
		OrderBy:    "total_cost",
		Descending: true,
	})
	if err != nil {
		t.Fatalf("query page 1 failed: %v", err)
	}
	if len(res1.Traces) != 60 {
		t.Fatalf("page 1 traces = %d, want 60", len(res1.Traces))
	}
	if res1.NextPageToken == "" {
		t.Fatal("page 1 expected non-empty NextPageToken")
	}
	if res1.Traces[0].TraceID != "trace-099" {
		t.Errorf("page 1 first trace = %s, want trace-099", res1.Traces[0].TraceID)
	}

	res2, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID:  "proj-cost",
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(240 * time.Minute),
		PageSize:   60,
		OrderBy:    "total_cost",
		Descending: true,
		PageToken:  res1.NextPageToken,
	})
	if err != nil {
		t.Fatalf("query page 2 failed: %v", err)
	}
	if len(res2.Traces) != 40 {
		t.Fatalf("page 2 traces = %d, want 40", len(res2.Traces))
	}
	if res2.NextPageToken != "" {
		t.Fatal("page 2 expected empty NextPageToken")
	}

	seen := make(map[string]bool)
	for _, tr := range res1.Traces {
		seen[tr.TraceID] = true
	}
	for _, tr := range res2.Traces {
		if seen[tr.TraceID] {
			t.Errorf("duplicate trace found across pages: %s", tr.TraceID)
		}
		seen[tr.TraceID] = true
	}
	if len(seen) != 100 {
		t.Errorf("combined unique traces = %d, want 100", len(seen))
	}
}

func TestQueryTraces_NoDuplicates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	var spans []storage.Span
	// Use same timestamp to test tiebreaker
	ts := now
	for i := 0; i < 100; i++ {
		traceID := fmt.Sprintf("trace-%03d", i)
		spans = append(spans, storage.Span{
			SpanID:    "span-" + traceID,
			TraceID:   traceID,
			Name:      "root",
			Kind:      storage.SpanKindLLM,
			Status:    storage.SpanStatusOK,
			StartTime: ts,
			EndTime:   ts.Add(time.Second),
			Duration:  time.Second,
			ProjectID: "proj-1",
			GenAI:     &storage.GenAIAttributes{Model: "gpt-4o"},
		})
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	// Page 1
	res1, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  60,
	})
	if err != nil {
		t.Fatalf("query page 1 failed: %v", err)
	}
	if len(res1.Traces) != 60 {
		t.Fatalf("page 1 traces = %d, want 60", len(res1.Traces))
	}
	if res1.NextPageToken == "" {
		t.Fatal("page 1 expected non-empty NextPageToken")
	}

	// Page 2
	res2, err := s.QueryTraces(ctx, storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		PageSize:  60,
		PageToken: res1.NextPageToken,
	})
	if err != nil {
		t.Fatalf("query page 2 failed: %v", err)
	}
	if len(res2.Traces) != 40 {
		t.Fatalf("page 2 traces = %d, want 40", len(res2.Traces))
	}
	if res2.NextPageToken != "" {
		t.Fatal("page 2 expected empty NextPageToken")
	}

	seen := make(map[string]bool)
	for _, tr := range res1.Traces {
		seen[tr.TraceID] = true
	}
	for _, tr := range res2.Traces {
		if seen[tr.TraceID] {
			t.Errorf("duplicate trace found across pages: %s", tr.TraceID)
		}
		seen[tr.TraceID] = true
	}
	if len(seen) != 100 {
		t.Errorf("combined unique traces = %d, want 100", len(seen))
	}
}
