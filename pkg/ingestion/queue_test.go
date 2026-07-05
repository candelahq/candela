package ingestion_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/ingestion"
	"github.com/candelahq/candela/pkg/storage"
)

// chanQueue is an in-memory implementation of ingestion.Queue backed by a
// buffered channel of span batches. It is intended only for testing.
type chanQueue struct {
	ch   chan []storage.Span
	once sync.Once
}

func newChanQueue(capacity int) *chanQueue {
	return &chanQueue{ch: make(chan []storage.Span, capacity)}
}

func (q *chanQueue) Push(ctx context.Context, spans []storage.Span) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case q.ch <- spans:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *chanQueue) Pull(ctx context.Context, batchSize int) ([]storage.Span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case batch, ok := <-q.ch:
		if !ok {
			return nil, nil
		}
		if len(batch) > batchSize {
			batch = batch[:batchSize]
		}
		return batch, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *chanQueue) Ping(_ context.Context) error {
	return nil
}

func (q *chanQueue) Close() error {
	q.once.Do(func() {
		close(q.ch)
	})
	return nil
}

// Compile-time interface satisfaction check.
var _ ingestion.Queue = (*chanQueue)(nil)

// makeSpan creates a minimal storage.Span with the given SpanID and reasonable
// defaults for the remaining required fields.
func makeSpan(id string) storage.Span {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return storage.Span{
		SpanID:    id,
		TraceID:   "trace-" + id,
		Name:      "span-" + id,
		Kind:      storage.SpanKindGeneral,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		ProjectID: "test-project",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestQueue_PushAndPull(t *testing.T) {
	q := newChanQueue(10)
	ctx := context.Background()

	spans := []storage.Span{makeSpan("1"), makeSpan("2"), makeSpan("3")}

	if err := q.Push(ctx, spans); err != nil {
		t.Fatalf("Push: unexpected error: %v", err)
	}

	got, err := q.Pull(ctx, 10)
	if err != nil {
		t.Fatalf("Pull: unexpected error: %v", err)
	}

	if len(got) != len(spans) {
		t.Fatalf("Pull: got %d spans, want %d", len(got), len(spans))
	}
	for i, s := range got {
		if s.SpanID != spans[i].SpanID {
			t.Errorf("span[%d].SpanID = %q, want %q", i, s.SpanID, spans[i].SpanID)
		}
	}
}

func TestQueue_PullBatchSize(t *testing.T) {
	q := newChanQueue(10)
	ctx := context.Background()

	spans := []storage.Span{
		makeSpan("a"), makeSpan("b"), makeSpan("c"),
		makeSpan("d"), makeSpan("e"),
	}
	if err := q.Push(ctx, spans); err != nil {
		t.Fatalf("Push: unexpected error: %v", err)
	}

	got, err := q.Pull(ctx, 3)
	if err != nil {
		t.Fatalf("Pull: unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Pull with batchSize=3: got %d spans, want 3", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].SpanID != want {
			t.Errorf("span[%d].SpanID = %q, want %q", i, got[i].SpanID, want)
		}
	}
}

func TestQueue_EmptyPullCancelled(t *testing.T) {
	q := newChanQueue(10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := q.Pull(ctx, 10)
	if err == nil {
		t.Fatal("Pull on empty queue with cancelled context: expected error, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("Pull error = %v, want %v", err, context.Canceled)
	}
}

func TestQueue_PushFullQueueCancelled(t *testing.T) {
	q := newChanQueue(1)
	ctx := context.Background()

	// Fill the queue.
	if err := q.Push(ctx, []storage.Span{makeSpan("fill")}); err != nil {
		t.Fatalf("initial Push: unexpected error: %v", err)
	}

	// Now push with a cancelled context — should fail.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Push(cancelledCtx, []storage.Span{makeSpan("overflow")})
	if err == nil {
		t.Fatal("Push to full queue with cancelled context: expected error, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("Push error = %v, want %v", err, context.Canceled)
	}
}

// TestQueue_PullBlockingCancelled verifies that a Pull blocked in the select
// statement (empty queue, live context) returns promptly when the context is
// cancelled — exercising the ctx.Done() branch inside select, not the pre-check.
func TestQueue_PullBlockingCancelled(t *testing.T) {
	q := newChanQueue(10)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := q.Pull(ctx, 10)
		errCh <- err
	}()

	// Give the goroutine time to block in the select.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("Pull error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Pull did not return after context cancellation")
	}
}

// TestQueue_PushBlockingCancelled verifies that a Push blocked in the select
// statement (full queue, live context) returns promptly when the context is
// cancelled — exercising the ctx.Done() branch inside select, not the pre-check.
func TestQueue_PushBlockingCancelled(t *testing.T) {
	q := newChanQueue(1)
	ctx := context.Background()

	// Fill the queue.
	if err := q.Push(ctx, []storage.Span{makeSpan("fill")}); err != nil {
		t.Fatalf("initial Push: unexpected error: %v", err)
	}

	pushCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- q.Push(pushCtx, []storage.Span{makeSpan("blocked")})
	}()

	// Give the goroutine time to block in the select.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("Push error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Push did not return after context cancellation")
	}
}

func TestQueue_ConcurrentPushPull(t *testing.T) {
	const (
		numProducers  = 5
		spansPerBatch = 20
	)

	q := newChanQueue(numProducers * 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Producers: each pushes one batch.
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			batch := make([]storage.Span, spansPerBatch)
			for i := range batch {
				batch[i] = makeSpan(fmt.Sprintf("%s-%c-%d", t.Name(), 'A'+rune(producerID), i))
			}
			if err := q.Push(ctx, batch); err != nil {
				t.Errorf("producer %d Push: %v", producerID, err)
			}
		}(p)
	}

	// Consumer: pull all batches.
	var mu sync.Mutex
	var collected []storage.Span
	for c := 0; c < numProducers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := q.Pull(ctx, spansPerBatch)
			if err != nil {
				t.Errorf("Pull: %v", err)
				return
			}
			mu.Lock()
			collected = append(collected, got...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	totalExpected := numProducers * spansPerBatch
	if len(collected) != totalExpected {
		t.Errorf("collected %d spans, want %d", len(collected), totalExpected)
	}
}

func TestQueue_PingAndClose(t *testing.T) {
	q := newChanQueue(1)

	if err := q.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: unexpected error: %v", err)
	}

	if err := q.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
}

func TestQueue_PushNilSpans(t *testing.T) {
	q := newChanQueue(10)
	ctx := context.Background()

	// Push nil slice.
	if err := q.Push(ctx, nil); err != nil {
		t.Fatalf("Push(nil): unexpected error: %v", err)
	}
	got, err := q.Pull(ctx, 10)
	if err != nil {
		t.Fatalf("Pull after nil push: unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Pull after nil push: got %v, want nil", got)
	}

	// Push empty slice.
	if err := q.Push(ctx, []storage.Span{}); err != nil {
		t.Fatalf("Push(empty): unexpected error: %v", err)
	}
	got, err = q.Pull(ctx, 10)
	if err != nil {
		t.Fatalf("Pull after empty push: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Pull after empty push: got %d spans, want 0", len(got))
	}
}

func TestQueue_MultipleBatches(t *testing.T) {
	q := newChanQueue(10)
	ctx := context.Background()

	batches := [][]storage.Span{
		{makeSpan("batch1-a"), makeSpan("batch1-b")},
		{makeSpan("batch2-a")},
		{makeSpan("batch3-a"), makeSpan("batch3-b"), makeSpan("batch3-c")},
	}

	for i, batch := range batches {
		if err := q.Push(ctx, batch); err != nil {
			t.Fatalf("Push batch %d: unexpected error: %v", i, err)
		}
	}

	for i, want := range batches {
		got, err := q.Pull(ctx, 100)
		if err != nil {
			t.Fatalf("Pull batch %d: unexpected error: %v", i, err)
		}
		if len(got) != len(want) {
			t.Fatalf("batch %d: got %d spans, want %d", i, len(got), len(want))
		}
		for j, s := range got {
			if s.SpanID != want[j].SpanID {
				t.Errorf("batch %d span[%d].SpanID = %q, want %q", i, j, s.SpanID, want[j].SpanID)
			}
		}
	}
}
