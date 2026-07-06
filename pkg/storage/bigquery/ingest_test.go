package bigquery

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"

	"github.com/candelahq/candela/pkg/storage"
)

// makeSpan creates a minimal test span with configurable retry attribute.
func makeSpan(id string, isRetry bool) storage.Span {
	attrs := map[string]string{}
	if isRetry {
		attrs["candela.is_retry"] = "true"
	}
	return storage.Span{
		SpanID:     "span-" + id,
		TraceID:    "trace-" + id,
		Name:       "test.span." + id,
		StartTime:  time.Now().Add(-1 * time.Second),
		EndTime:    time.Now(),
		Attributes: attrs,
	}
}

// ── Optimistic Path ──────────────────────────────────────────────────

func TestIngestSpans_OptimisticPath(t *testing.T) {
	client := newMockBQClient()
	inserter := &mockBQInserter{}
	client.dataset.tables["spans"] = &mockBQTable{id: "spans", inserter: inserter}

	s := testStore(client)
	spans := []storage.Span{makeSpan("1", false), makeSpan("2", false)}

	err := s.IngestSpans(context.Background(), spans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !inserter.putCalled {
		t.Fatal("expected inserter.Put to be called")
	}

	// Verify the rows passed to Put are StructSavers.
	rows, ok := inserter.putSrc.([]*bq.StructSaver)
	if !ok {
		t.Fatalf("expected []*bigquery.StructSaver, got %T", inserter.putSrc)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
	// Verify dedup key format: traceID-spanID.
	for _, r := range rows {
		if !strings.Contains(r.InsertID, "-span-") {
			t.Errorf("unexpected InsertID format: %s", r.InsertID)
		}
	}
}

// ── Pessimistic Path ─────────────────────────────────────────────────

func TestIngestSpans_PessimisticPath(t *testing.T) {
	client := newMockBQClient()
	inserter := &mockBQInserter{}
	client.dataset.tables["spans"] = &mockBQTable{id: "spans", inserter: inserter}

	mergeQuery := &mockBQQuery{job: &mockBQJob{}}
	client.enqueueQuery(mergeQuery)

	s := testStore(client)
	spans := []storage.Span{makeSpan("1", true), makeSpan("2", true)}

	err := s.IngestSpans(context.Background(), spans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Inserter should NOT be called (all pessimistic).
	if inserter.putCalled {
		t.Error("inserter.Put should not be called for pessimistic spans")
	}

	// Verify MERGE SQL.
	assertContains(t, mergeQuery.sql, "MERGE")
	assertContains(t, mergeQuery.sql, "WHEN NOT MATCHED THEN")
	assertParam(t, mergeQuery.params, "spans")
}

// ── Mixed Batch ──────────────────────────────────────────────────────

func TestIngestSpans_MixedBatch(t *testing.T) {
	client := newMockBQClient()
	inserter := &mockBQInserter{}
	client.dataset.tables["spans"] = &mockBQTable{id: "spans", inserter: inserter}

	mergeQuery := &mockBQQuery{job: &mockBQJob{}}
	client.enqueueQuery(mergeQuery)

	s := testStore(client)
	spans := []storage.Span{
		makeSpan("opt-1", false),
		makeSpan("pess-1", true),
		makeSpan("opt-2", false),
	}

	err := s.IngestSpans(context.Background(), spans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Optimistic: inserter called with 2 rows.
	if !inserter.putCalled {
		t.Fatal("expected inserter.Put to be called for optimistic spans")
	}
	rows, ok := inserter.putSrc.([]*bq.StructSaver)
	if !ok {
		t.Fatalf("expected []*bigquery.StructSaver, got %T", inserter.putSrc)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 optimistic rows, got %d", len(rows))
	}

	// Pessimistic: MERGE query called with 1 span.
	assertContains(t, mergeQuery.sql, "MERGE")
	assertParam(t, mergeQuery.params, "spans")
}

// ── Insert Error ─────────────────────────────────────────────────────

func TestIngestSpans_InsertError(t *testing.T) {
	client := newMockBQClient()
	inserter := &mockBQInserter{putErr: fmt.Errorf("quota exceeded")}
	client.dataset.tables["spans"] = &mockBQTable{id: "spans", inserter: inserter}

	s := testStore(client)
	err := s.IngestSpans(context.Background(), []storage.Span{makeSpan("1", false)})

	if err == nil {
		t.Fatal("expected error from IngestSpans")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("error = %v, want to contain 'quota exceeded'", err)
	}
	if !strings.Contains(err.Error(), "optimistic") {
		t.Errorf("error = %v, want to contain 'optimistic'", err)
	}
}

// ── Merge Error ──────────────────────────────────────────────────────

func TestIngestSpans_MergeError(t *testing.T) {
	client := newMockBQClient()
	inserter := &mockBQInserter{}
	client.dataset.tables["spans"] = &mockBQTable{id: "spans", inserter: inserter}

	mergeQuery := &mockBQQuery{
		job: &mockBQJob{waitErr: fmt.Errorf("deadline exceeded")},
	}
	client.enqueueQuery(mergeQuery)

	s := testStore(client)
	err := s.IngestSpans(context.Background(), []storage.Span{makeSpan("1", true)})

	if err == nil {
		t.Fatal("expected error from IngestSpans")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("error = %v, want to contain 'deadline exceeded'", err)
	}
	if !strings.Contains(err.Error(), "pessimistic") {
		t.Errorf("error = %v, want to contain 'pessimistic'", err)
	}
}

// ── Empty Input ──────────────────────────────────────────────────────

func TestIngestSpans_EmptyInput(t *testing.T) {
	client := newMockBQClient()
	s := testStore(client)

	err := s.IngestSpans(context.Background(), []storage.Span{})
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}

	err = s.IngestSpans(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error for nil input: %v", err)
	}
}
