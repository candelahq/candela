package spendoutbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockUserStore records DeductSpend calls and can be configured to fail.
type mockUserStore struct {
	mu    sync.Mutex
	calls []deductCall
	err   error // if non-nil, DeductSpend returns this error
}

type deductCall struct {
	UserID  string
	CostUSD float64
	Tokens  int64
}

func (m *mockUserStore) DeductSpend(_ context.Context, userID string, costUSD float64, tokens int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, deductCall{UserID: userID, CostUSD: costUSD, Tokens: tokens})
	return m.err
}

func (m *mockUserStore) DeductTaskSpend(_ context.Context, _ string, _ float64) error {
	return nil
}

func (m *mockUserStore) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func TestSpendSyncWorker_HappyPath(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()
	mock := &mockUserStore{}

	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "happy-1",
		UserID:  "user-abc",
		CostUSD: 0.05,
		Tokens:  500,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	w := NewSpendSyncWorker(ob, mock, 100*time.Millisecond)
	w.Start()
	time.Sleep(350 * time.Millisecond)
	w.Stop()

	// Verify DeductSpend was called.
	if got := mock.callCount(); got == 0 {
		t.Fatal("DeductSpend was never called")
	}

	// Verify record was deleted from the outbox.
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 0 {
		t.Errorf("pending = %d, want 0 (record should be deleted after success)", count)
	}
}

func TestSpendSyncWorker_RetryOnFailure(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()
	mock := &mockUserStore{err: errors.New("firestore unavailable")}

	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "retry-1",
		UserID:  "user-xyz",
		CostUSD: 1.00,
		Tokens:  1000,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	w := NewSpendSyncWorker(ob, mock, 100*time.Millisecond)
	w.Start()
	time.Sleep(350 * time.Millisecond)
	w.Stop()

	// Record should still be pending.
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 1 {
		t.Errorf("pending = %d, want 1 (record should remain after failure)", count)
	}

	// DeductSpend should have been called at least once (first tick).
	if got := mock.callCount(); got < 1 {
		t.Errorf("DeductSpend called %d times, want >= 1", got)
	}
}

func TestSpendSyncWorker_MaxAgeExpiry(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()
	mock := &mockUserStore{}

	// Enqueue a record with a very old created_at.
	if err := ob.Enqueue(ctx, SpendRecord{
		ID:        "old-1",
		UserID:    "user-old",
		CostUSD:   2.50,
		Tokens:    3000,
		CreatedAt: time.Now().UTC().Add(-48 * time.Hour), // 2 days ago, beyond 24h maxAge
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	w := NewSpendSyncWorker(ob, mock, 100*time.Millisecond)
	w.Start()
	time.Sleep(350 * time.Millisecond)
	w.Stop()

	// DeductSpend should NOT have been called (permanent failure path skips it).
	if got := mock.callCount(); got != 0 {
		t.Errorf("DeductSpend called %d times, want 0 (should be dropped as permanent failure)", got)
	}

	// Record should be deleted.
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 0 {
		t.Errorf("pending = %d, want 0 (expired record should be deleted)", count)
	}
}

func TestSpendSyncWorker_RetryLaterUsed(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()
	mock := &mockUserStore{err: errors.New("transient error")}

	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "rl-1",
		UserID:  "user-rl",
		CostUSD: 0.50,
		Tokens:  500,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	w := NewSpendSyncWorker(ob, mock, 100*time.Millisecond)
	w.Start()
	// Wait for one tick to process.
	time.Sleep(250 * time.Millisecond)
	w.Stop()

	// Record should still be pending (failure, not dropped).
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 1 {
		t.Errorf("pending = %d, want 1", count)
	}

	// After RetryLater, next_retry_at should be in the future, so Peek returns nothing.
	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Peek returned %d records, want 0 (next_retry_at should be in the future)", len(records))
	}
}
