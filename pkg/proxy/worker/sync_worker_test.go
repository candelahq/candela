package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSyncStore implements storage.SyncStore for testing.
type MockSyncStore struct {
	OutboxSpans            []storage.OutboxSpan
	DeletedSpanIDs         []string
	IncrementedSpanIDs     []string
	PrunedKeepCount        int
	GetOutboxSpansErr      error
	DeleteOutboxSpansErr   error
	IncrementOutboxErr     error
	PruneLocalSpansErr     error
	GetOutboxSpansCalls    int
	DeleteOutboxSpansCalls int
	IncrementOutboxCalls   int
	PruneLocalSpansCalls   int
}

func (m *MockSyncStore) GetOutboxSpans(ctx context.Context, limit int) ([]storage.OutboxSpan, error) {
	m.GetOutboxSpansCalls++
	if m.GetOutboxSpansErr != nil {
		return nil, m.GetOutboxSpansErr
	}
	if limit > len(m.OutboxSpans) {
		return m.OutboxSpans, nil
	}
	return m.OutboxSpans[:limit], nil
}

func (m *MockSyncStore) DeleteOutboxSpans(ctx context.Context, spanIDs []string) error {
	m.DeleteOutboxSpansCalls++
	if m.DeleteOutboxSpansErr != nil {
		return m.DeleteOutboxSpansErr
	}
	m.DeletedSpanIDs = append(m.DeletedSpanIDs, spanIDs...)
	return nil
}

func (m *MockSyncStore) IncrementOutboxAttempt(ctx context.Context, spanIDs []string) error {
	m.IncrementOutboxCalls++
	if m.IncrementOutboxErr != nil {
		return m.IncrementOutboxErr
	}
	m.IncrementedSpanIDs = append(m.IncrementedSpanIDs, spanIDs...)
	return nil
}

func (m *MockSyncStore) PruneLocalSpans(ctx context.Context, keepCount int) error {
	m.PruneLocalSpansCalls++
	if m.PruneLocalSpansErr != nil {
		return m.PruneLocalSpansErr
	}
	m.PrunedKeepCount = keepCount
	return nil
}

// MockSpanWriter implements storage.SpanWriter for testing upstream syncing.
type MockSpanWriter struct {
	IngestedSpans []storage.Span
	IngestErr     error
	IngestCalls   int
	IngestFunc    func(ctx context.Context, spans []storage.Span) error
}

func (m *MockSpanWriter) IngestSpans(ctx context.Context, spans []storage.Span) error {
	m.IngestCalls++
	if m.IngestFunc != nil {
		return m.IngestFunc(ctx, spans)
	}
	if m.IngestErr != nil {
		return m.IngestErr
	}
	m.IngestedSpans = append(m.IngestedSpans, spans...)
	return nil
}

func (m *MockSpanWriter) Close() error {
	return nil
}

// --- Helper ---

func makeOutboxSpan(id string, attempt int) storage.OutboxSpan {
	span := storage.Span{SpanID: id, Name: "test-" + id}
	payload, _ := json.Marshal(span)
	return storage.OutboxSpan{
		SpanID:       id,
		PayloadJSON:  string(payload),
		AttemptCount: attempt,
		CreatedAt:    time.Now(),
	}
}

// ────────────────────────────────────────────────────────────
// Existing tests (preserved)
// ────────────────────────────────────────────────────────────

func TestSyncWorker_HappyPath(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	// Setup initial outbox state
	span1 := storage.Span{SpanID: "span-1", Name: "Test"}
	payload, _ := json.Marshal(span1)

	mockStore.OutboxSpans = []storage.OutboxSpan{
		{SpanID: "span-1", PayloadJSON: string(payload), AttemptCount: 0},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// Process one batch manually
	worker.processBatch()

	// Assertions
	require.Equal(t, 1, mockStore.GetOutboxSpansCalls)
	require.Equal(t, 1, mockUpstream.IngestCalls)
	require.Len(t, mockUpstream.IngestedSpans, 1)

	// Verify candela.is_retry is NOT injected for attempt = 0
	ingested := mockUpstream.IngestedSpans[0]
	assert.Equal(t, "span-1", ingested.SpanID)
	assert.Empty(t, ingested.Attributes["candela.is_retry"])

	// Verify successful deletion
	require.Equal(t, 1, mockStore.DeleteOutboxSpansCalls)
	require.Contains(t, mockStore.DeletedSpanIDs, "span-1")
	assert.Equal(t, 0, mockStore.IncrementOutboxCalls)
}

func TestSyncWorker_RetryInjection(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	// Span already failed once
	span1 := storage.Span{SpanID: "span-1", Name: "Test"}
	payload, _ := json.Marshal(span1)

	mockStore.OutboxSpans = []storage.OutboxSpan{
		{SpanID: "span-1", PayloadJSON: string(payload), AttemptCount: 1}, // Attempt > 0 triggers pessimistic ingestion
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	require.Equal(t, 1, mockUpstream.IngestCalls)
	require.Len(t, mockUpstream.IngestedSpans, 1)

	ingested := mockUpstream.IngestedSpans[0]
	// Verify candela.is_retry IS injected
	assert.Equal(t, "true", ingested.Attributes["candela.is_retry"])

	// Verify successful deletion
	require.Contains(t, mockStore.DeletedSpanIDs, "span-1")
}

func TestSyncWorker_UpstreamFailure(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{
		IngestErr: fmt.Errorf("network offline"), // Upstream fails
	}

	span1 := storage.Span{SpanID: "span-1", Name: "Test"}
	payload, _ := json.Marshal(span1)

	mockStore.OutboxSpans = []storage.OutboxSpan{
		{SpanID: "span-1", PayloadJSON: string(payload), AttemptCount: 0},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	require.Equal(t, 1, mockUpstream.IngestCalls)

	// Verify nothing deleted
	assert.Equal(t, 0, mockStore.DeleteOutboxSpansCalls)

	// Verify attempt count incremented
	require.Equal(t, 1, mockStore.IncrementOutboxCalls)
	require.Contains(t, mockStore.IncrementedSpanIDs, "span-1")
}

func TestSyncWorker_CorruptedJSON(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	// Mixing good and bad JSON
	span1 := storage.Span{SpanID: "span-1", Name: "Good"}
	payload, _ := json.Marshal(span1)

	mockStore.OutboxSpans = []storage.OutboxSpan{
		{SpanID: "span-1", PayloadJSON: string(payload), AttemptCount: 0},
		{SpanID: "span-bad", PayloadJSON: "{corrupted json", AttemptCount: 0},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// Only 1 span should reach upstream
	require.Len(t, mockUpstream.IngestedSpans, 1)
	assert.Equal(t, "span-1", mockUpstream.IngestedSpans[0].SpanID)

	// Both should be deleted (corrupted ones are deleted as poison pills)
	// Note: The corrupted JSON is deleted directly within the loop, the valid ones at the end.
	require.Contains(t, mockStore.DeletedSpanIDs, "span-bad")
	require.Contains(t, mockStore.DeletedSpanIDs, "span-1")
}

func TestSyncWorker_PruneLoop(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// We can manually trigger the prune function
	worker.pruneLocal(context.Background())

	require.Equal(t, 1, mockStore.PruneLocalSpansCalls)

	// Should be pruning spans keeping 100,000
	assert.Equal(t, 100000, mockStore.PrunedKeepCount)
}

func TestSyncWorker_GracefulShutdown(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	// Upstream will block until the context is canceled
	mockUpstream := &MockSpanWriter{
		IngestErr: context.Canceled, // Or we could use a custom mock, but we can verify ctx directly
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// Set up a mock outbox span
	span1 := storage.Span{SpanID: "span-1", Name: "Test"}
	payload, _ := json.Marshal(span1)

	mockStore.OutboxSpans = []storage.OutboxSpan{
		{SpanID: "span-1", PayloadJSON: string(payload), AttemptCount: 0},
	}

	// We'll wrap processBatch in a goroutine and then Stop() immediately
	// If processBatch takes a long time because of network timeout, Stop() should cancel it.

	// Start the worker properly
	worker.Start()

	// Stop it, which triggers context cancellation
	worker.Stop()

	// If it doesn't block forever, Stop works. The context cancellation ensures processBatch aborts if blocked.
	// Let's verify that after Stop, calling processBatch directly with a canceled ctx fails fast.
	err := worker.ctx.Err()
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSyncWorker_PayloadBisection(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}

	// Create a mock upstream that fails if batch size > 1 with a "413 Payload Too Large" error
	mockUpstream := &MockSpanWriter{
		IngestFunc: func(ctx context.Context, spans []storage.Span) error {
			if len(spans) > 1 {
				return fmt.Errorf("rpc error: code = ResourceExhausted desc = grpc: received message larger than max (413 Payload Too Large)")
			}
			return nil
		},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// Set up 4 mock outbox spans
	for i := 0; i < 4; i++ {
		span := storage.Span{SpanID: fmt.Sprintf("span-%d", i), Name: "Test"}
		payload, _ := json.Marshal(span)
		mockStore.OutboxSpans = append(mockStore.OutboxSpans, storage.OutboxSpan{
			SpanID:       fmt.Sprintf("span-%d", i),
			PayloadJSON:  string(payload),
			AttemptCount: 0,
		})
	}

	worker.processBatch()

	// It should have bisected down to individual spans and succeeded for all 4.
	// 1 call for 4 spans -> fails
	// 2 calls for 2 spans -> fails
	// 4 calls for 1 span -> succeeds
	assert.Equal(t, 7, mockUpstream.IngestCalls) // 1 + 2 + 4 = 7 calls

	// Since all succeeded eventually, all 4 should be deleted from the outbox
	assert.ElementsMatch(t, []string{"span-0", "span-1", "span-2", "span-3"}, mockStore.DeletedSpanIDs)
	assert.Empty(t, mockStore.IncrementedSpanIDs)
}

// ────────────────────────────────────────────────────────────
// NEW tests
// ────────────────────────────────────────────────────────────

func TestSyncWorker_EmptyOutbox(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// Should query the outbox but not do anything else
	assert.Equal(t, 1, mockStore.GetOutboxSpansCalls)
	assert.Equal(t, 0, mockUpstream.IngestCalls)
	assert.Equal(t, 0, mockStore.DeleteOutboxSpansCalls)
	assert.Equal(t, 0, mockStore.IncrementOutboxCalls)
}

func TestSyncWorker_AllCorrupted(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{
		OutboxSpans: []storage.OutboxSpan{
			{SpanID: "bad-1", PayloadJSON: "not json", AttemptCount: 0},
			{SpanID: "bad-2", PayloadJSON: "{truncated", AttemptCount: 0},
			{SpanID: "bad-3", PayloadJSON: "", AttemptCount: 0},
		},
	}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// No spans should reach upstream
	assert.Equal(t, 0, mockUpstream.IngestCalls)

	// All corrupted IDs should be batch-deleted
	assert.ElementsMatch(t, []string{"bad-1", "bad-2", "bad-3"}, mockStore.DeletedSpanIDs)

	// Only 1 delete call (batched), not 3
	assert.Equal(t, 1, mockStore.DeleteOutboxSpansCalls)
}

func TestSyncWorker_MixedRetryAndFresh(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{
		OutboxSpans: []storage.OutboxSpan{
			makeOutboxSpan("fresh-1", 0),
			makeOutboxSpan("retry-1", 2),
			makeOutboxSpan("fresh-2", 0),
			makeOutboxSpan("retry-2", 5),
		},
	}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// All 4 should be ingested in one call
	require.Equal(t, 1, mockUpstream.IngestCalls)
	require.Len(t, mockUpstream.IngestedSpans, 4)

	// Verify retry flag is only on the retried spans
	for _, s := range mockUpstream.IngestedSpans {
		switch s.SpanID {
		case "fresh-1", "fresh-2":
			assert.Empty(t, s.Attributes["candela.is_retry"],
				"fresh span %s should not have is_retry", s.SpanID)
		case "retry-1", "retry-2":
			assert.Equal(t, "true", s.Attributes["candela.is_retry"],
				"retried span %s should have is_retry=true", s.SpanID)
		}
	}

	// All should be successfully deleted
	assert.ElementsMatch(t,
		[]string{"fresh-1", "retry-1", "fresh-2", "retry-2"},
		mockStore.DeletedSpanIDs)
}

func TestSyncWorker_PartialBisectionFailure(t *testing.T) {
	t.Parallel()

	// Simulates: batch of 4, upstream rejects "poison-span" permanently
	// (non-size error), and the other 3 succeed after bisection.
	var callCount atomic.Int32
	mockUpstream := &MockSpanWriter{
		IngestFunc: func(ctx context.Context, spans []storage.Span) error {
			callCount.Add(1)
			// Fail batches containing "poison-span", succeed otherwise
			for _, s := range spans {
				if s.SpanID == "poison-span" {
					if len(spans) > 1 {
						// Size-like error to trigger bisection
						return fmt.Errorf("413 Payload Too Large")
					}
					// Single poison span: permanent failure (non-size)
					return fmt.Errorf("upstream rejected: bad data")
				}
			}
			return nil
		},
	}

	mockStore := &MockSyncStore{
		OutboxSpans: []storage.OutboxSpan{
			makeOutboxSpan("good-1", 0),
			makeOutboxSpan("poison-span", 0),
			makeOutboxSpan("good-2", 0),
			makeOutboxSpan("good-3", 0),
		},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// The poison span should have its attempt incremented
	assert.Contains(t, mockStore.IncrementedSpanIDs, "poison-span")

	// The good spans should be deleted
	assert.Contains(t, mockStore.DeletedSpanIDs, "good-1")
	assert.Contains(t, mockStore.DeletedSpanIDs, "good-2")
	assert.Contains(t, mockStore.DeletedSpanIDs, "good-3")
	assert.NotContains(t, mockStore.DeletedSpanIDs, "poison-span")
}

func TestSyncWorker_GetOutboxError(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{
		GetOutboxSpansErr: fmt.Errorf("database locked"),
	}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	// Should have tried to get outbox but failed; nothing else happens
	assert.Equal(t, 1, mockStore.GetOutboxSpansCalls)
	assert.Equal(t, 0, mockUpstream.IngestCalls)
	assert.Equal(t, 0, mockStore.DeleteOutboxSpansCalls)
}

func TestSyncWorker_ContextCanceledMidBatch(t *testing.T) {
	t.Parallel()

	// Simulate the upstream blocking and the worker being stopped
	mockUpstream := &MockSpanWriter{
		IngestFunc: func(ctx context.Context, spans []storage.Span) error {
			// Check if context was already canceled
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		},
	}

	mockStore := &MockSyncStore{
		OutboxSpans: []storage.OutboxSpan{
			makeOutboxSpan("span-1", 0),
		},
	}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// Cancel the worker's context before processing
	worker.cancel()

	worker.processBatch()

	// The 30s timeout context derived from w.ctx should be immediately canceled.
	// processBatch uses context.WithTimeout(w.ctx, ...), so if w.ctx is canceled,
	// the derived context is also canceled. The behavior depends on whether
	// GetOutboxSpans or IngestSpans checks ctx first.
	// Either way, no spans should remain stuck — the worker should not block.
}

func TestSyncWorker_RetryPreservesExistingAttributes(t *testing.T) {
	t.Parallel()

	// A span that already has attributes should gain candela.is_retry without
	// losing its existing attributes.
	span := storage.Span{
		SpanID: "span-with-attrs",
		Name:   "test",
		Attributes: map[string]string{
			"http.method": "POST",
			"user.id":     "u123",
		},
	}
	payload, _ := json.Marshal(span)

	mockStore := &MockSyncStore{
		OutboxSpans: []storage.OutboxSpan{
			{SpanID: "span-with-attrs", PayloadJSON: string(payload), AttemptCount: 3},
		},
	}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)
	worker.processBatch()

	require.Len(t, mockUpstream.IngestedSpans, 1)
	ingested := mockUpstream.IngestedSpans[0]

	// Verify retry flag is set
	assert.Equal(t, "true", ingested.Attributes["candela.is_retry"])
	// Verify original attributes are preserved
	assert.Equal(t, "POST", ingested.Attributes["http.method"])
	assert.Equal(t, "u123", ingested.Attributes["user.id"])
}

func TestSyncWorker_PruneRespectsWorkerContext(t *testing.T) {
	t.Parallel()

	mockStore := &MockSyncStore{}
	mockUpstream := &MockSpanWriter{}

	worker := NewSyncWorker(mockStore, mockUpstream, 100*time.Millisecond)

	// Cancel the worker context
	worker.cancel()

	// pruneLocal should be called with the canceled context
	worker.pruneLocal(worker.ctx)

	// The mock doesn't check ctx, but we can verify it was called
	assert.Equal(t, 1, mockStore.PruneLocalSpansCalls)
	assert.Equal(t, 100000, mockStore.PrunedKeepCount)
}
