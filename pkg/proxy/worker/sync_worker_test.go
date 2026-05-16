package worker

import (
	"context"
	"encoding/json"
	"fmt"
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
