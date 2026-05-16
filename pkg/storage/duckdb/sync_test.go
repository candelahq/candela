package duckdb

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuckDB_DualWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	spans := []storage.Span{
		{
			SpanID:    "span-1",
			TraceID:   "trace-1",
			Name:      "test.span",
			StartTime: now,
			EndTime:   now.Add(time.Second),
		},
	}

	err := s.IngestSpans(ctx, spans)
	require.NoError(t, err)

	// Verify the span exists in outbox
	outbox, err := s.GetOutboxSpans(ctx, 10)
	require.NoError(t, err)
	require.Len(t, outbox, 1)

	assert.Equal(t, "span-1", outbox[0].SpanID)
	assert.Equal(t, 0, outbox[0].AttemptCount)
	assert.NotEmpty(t, outbox[0].PayloadJSON)
}

func TestDuckDB_OutboxOperations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	spans := []storage.Span{
		{SpanID: "span-1", TraceID: "trace-1", Name: "test.1", StartTime: now},
		{SpanID: "span-2", TraceID: "trace-1", Name: "test.2", StartTime: now},
		{SpanID: "span-3", TraceID: "trace-1", Name: "test.3", StartTime: now},
	}

	err := s.IngestSpans(ctx, spans)
	require.NoError(t, err)

	outbox, err := s.GetOutboxSpans(ctx, 10)
	require.NoError(t, err)
	require.Len(t, outbox, 3)

	// 1. Increment attempts for span-1 and span-2
	err = s.IncrementOutboxAttempt(ctx, []string{"span-1", "span-2"})
	require.NoError(t, err)

	// Verify incremented
	outbox, err = s.GetOutboxSpans(ctx, 10)
	require.NoError(t, err)

	attempts := make(map[string]int)
	for _, obs := range outbox {
		attempts[obs.SpanID] = obs.AttemptCount
	}
	assert.Equal(t, 1, attempts["span-1"])
	assert.Equal(t, 1, attempts["span-2"])
	assert.Equal(t, 0, attempts["span-3"])

	// 2. Delete span-3
	err = s.DeleteOutboxSpans(ctx, []string{"span-3"})
	require.NoError(t, err)

	// Verify deleted
	outbox, err = s.GetOutboxSpans(ctx, 10)
	require.NoError(t, err)
	require.Len(t, outbox, 2)
	assert.NotEqual(t, "span-3", outbox[0].SpanID)
	assert.NotEqual(t, "span-3", outbox[1].SpanID)
}

func TestDuckDB_Prune(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{SpanID: "span-old", TraceID: "trace-1", Name: "test.old", StartTime: now.Add(-2 * time.Hour)},
		{SpanID: "span-mid", TraceID: "trace-2", Name: "test.mid", StartTime: now.Add(-1 * time.Hour)},
		{SpanID: "span-new", TraceID: "trace-3", Name: "test.new", StartTime: now},
	}

	err := s.IngestSpans(ctx, spans)
	require.NoError(t, err)

	// Verify all 3 are present via GetTrace
	_, err = s.GetTrace(ctx, "trace-1")
	require.NoError(t, err)
	_, err = s.GetTrace(ctx, "trace-3")
	require.NoError(t, err)

	// Prune traces keeping only the 2 most recent
	err = s.PruneLocalSpans(ctx, 2)
	require.NoError(t, err)

	// Verification
	// Trace 1 (oldest) should be gone
	_, err = s.GetTrace(ctx, "trace-1")
	assert.Error(t, err) // Expect not found error

	// Trace 2 and 3 should still exist
	tr, err := s.GetTrace(ctx, "trace-2")
	require.NoError(t, err)
	assert.Equal(t, "trace-2", tr.TraceID)

	tr, err = s.GetTrace(ctx, "trace-3")
	require.NoError(t, err)
	assert.Equal(t, "trace-3", tr.TraceID)
}

func TestDuckDB_DeleteOutboxSpans_Chunking(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Ingest 1500 spans
	var spans []storage.Span
	var ids []string
	now := time.Now().UTC()
	for i := 0; i < 1500; i++ {
		id := fmt.Sprintf("span-%d", i)
		spans = append(spans, storage.Span{
			SpanID:    id,
			TraceID:   "trace-chunking",
			Name:      "test.chunk",
			StartTime: now,
		})
		ids = append(ids, id)
	}

	err := s.IngestSpans(ctx, spans)
	require.NoError(t, err)

	outbox, err := s.GetOutboxSpans(ctx, 2000)
	require.NoError(t, err)
	require.Len(t, outbox, 1500)

	err = s.IncrementOutboxAttempt(ctx, ids)
	require.NoError(t, err)

	err = s.DeleteOutboxSpans(ctx, ids)
	require.NoError(t, err)

	outbox, err = s.GetOutboxSpans(ctx, 2000)
	require.NoError(t, err)
	require.Empty(t, outbox)
}

func TestDuckDB_ConcurrentPruneAndWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed with a decent number of spans
	var initialSpans []storage.Span
	now := time.Now().UTC()
	for i := 0; i < 1000; i++ {
		initialSpans = append(initialSpans, storage.Span{
			SpanID:    fmt.Sprintf("span-initial-%d", i),
			TraceID:   "trace-initial",
			Name:      "test.initial",
			StartTime: now.Add(time.Duration(-i) * time.Minute),
		})
	}
	err := s.IngestSpans(ctx, initialSpans)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrently prune to small size
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := s.PruneLocalSpans(ctx, 100); err != nil {
				errCh <- fmt.Errorf("prune error: %w", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Concurrently write new spans
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			span := storage.Span{
				SpanID:    fmt.Sprintf("span-new-%d", i),
				TraceID:   "trace-new",
				Name:      "test.new",
				StartTime: time.Now().UTC(),
			}
			if err := s.IngestSpans(ctx, []storage.Span{span}); err != nil {
				errCh <- fmt.Errorf("write error: %w", err)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err, "Concurrent operation failed")
	}
}
