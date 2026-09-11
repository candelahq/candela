package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func BenchmarkQueryTraces(b *testing.B) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		b.Fatalf("opening duckdb: %v", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		b.Fatalf("migrating: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Pre-populate 100 traces with 5 spans each (500 spans).
	var spans []storage.Span
	models := []string{"gpt-4o", "claude-3-5-sonnet", "gemini-1.5-pro", "mistral-large"}
	for t := 0; t < 100; t++ {
		traceID := fmt.Sprintf("bench-trace-%04d", t)
		for sp := 0; sp < 5; sp++ {
			model := models[sp%len(models)]
			cost := float64(sp+1) * 0.002
			parentID := ""
			if sp > 0 {
				parentID = fmt.Sprintf("bench-span-%04d-0", t)
			}
			spans = append(spans, storage.Span{
				SpanID:       fmt.Sprintf("bench-span-%04d-%d", t, sp),
				TraceID:      traceID,
				ParentSpanID: parentID,
				Name:         fmt.Sprintf("call-%d", sp),
				Kind:         storage.SpanKindLLM,
				Status:       storage.SpanStatusOK,
				StartTime:    now.Add(time.Duration(sp*100) * time.Millisecond),
				EndTime:      now.Add(time.Duration(sp*100+50) * time.Millisecond),
				Duration:     50 * time.Millisecond,
				ProjectID:    "proj-bench",
				Environment:  "benchmark",
				TenantID:     "tenant-bench",
				GenAI: &storage.GenAIAttributes{
					Model:        model,
					Provider:     "openai",
					InputTokens:  200,
					OutputTokens: 100,
					TotalTokens:  300,
					CostUSD:      cost,
				},
			})
		}
	}

	if err := s.IngestSpans(ctx, spans); err != nil {
		b.Fatalf("ingesting spans: %v", err)
	}

	query := storage.TraceQuery{
		ProjectID: "proj-bench",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(24 * time.Hour),
		PageSize:  50,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := s.QueryTraces(ctx, query)
		if err != nil {
			b.Fatalf("QueryTraces error: %v", err)
		}
		if len(res.Traces) == 0 {
			b.Fatalf("expected traces, got 0")
		}
	}
}
