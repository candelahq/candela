package spendoutbox

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func newTestOutbox(t *testing.T) *Outbox {
	t.Helper()
	ob, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = ob.Close() })
	return ob
}

func TestEnqueue_and_Peek(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	rec := SpendRecord{
		ID:      "test-1",
		UserID:  "user-abc",
		CostUSD: 0.05,
		Tokens:  500,
	}
	if err := ob.Enqueue(ctx, rec); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	got := records[0]
	if got.ID != "test-1" {
		t.Errorf("ID = %q, want %q", got.ID, "test-1")
	}
	if got.UserID != "user-abc" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user-abc")
	}
	if got.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f, want 0.05", got.CostUSD)
	}
	if got.Tokens != 500 {
		t.Errorf("Tokens = %d, want 500", got.Tokens)
	}
	if got.AttemptCount != 0 {
		t.Errorf("AttemptCount = %d, want 0", got.AttemptCount)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestDelete(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := ob.Enqueue(ctx, SpendRecord{
			ID:      fmt.Sprintf("del-%d", i),
			UserID:  "user-1",
			CostUSD: 0.01,
			Tokens:  100,
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	// Delete the first two.
	if err := ob.Delete(ctx, []string{"del-0", "del-1"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].ID != "del-2" {
		t.Errorf("remaining ID = %q, want %q", records[0].ID, "del-2")
	}
}

func TestIncrementAttempt(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "inc-1",
		UserID:  "user-1",
		CostUSD: 1.0,
		Tokens:  1000,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := ob.IncrementAttempt(ctx, []string{"inc-1"}); err != nil {
		t.Fatalf("IncrementAttempt: %v", err)
	}
	if err := ob.IncrementAttempt(ctx, []string{"inc-1"}); err != nil {
		t.Fatalf("IncrementAttempt (2nd): %v", err)
	}

	// Small sleep so Peek's time.Now() is after IncrementAttempt's
	// next_retry_at. Under the race detector on slow CI, the two
	// time.Now() calls can be within nanoseconds of each other,
	// causing the Peek filter (next_retry_at <= now) to miss the record.
	time.Sleep(2 * time.Millisecond)

	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].AttemptCount != 2 {
		t.Errorf("AttemptCount = %d, want 2", records[0].AttemptCount)
	}
}

func TestPending(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	for i := 0; i < 5; i++ {
		if err := ob.Enqueue(ctx, SpendRecord{
			ID:      fmt.Sprintf("p-%d", i),
			UserID:  "u",
			CostUSD: 0.01,
			Tokens:  10,
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	count, err = ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestEnqueue_GeneratesID(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	if err := ob.Enqueue(ctx, SpendRecord{
		UserID:  "user-1",
		CostUSD: 0.10,
		Tokens:  200,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].ID == "" {
		t.Error("expected auto-generated ID, got empty string")
	}
	// 16 bytes → 32 hex chars.
	if len(records[0].ID) != 32 {
		t.Errorf("ID length = %d, want 32 hex chars", len(records[0].ID))
	}
}

func TestPeek_Ordering(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := ob.Enqueue(ctx, SpendRecord{
			ID:        fmt.Sprintf("ord-%d", i),
			UserID:    "user-1",
			CostUSD:   0.01,
			Tokens:    10,
			CreatedAt: now.Add(-time.Duration(i) * time.Second), // ord-0=now, ord-1=now-1s, ord-2=now-2s
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}

	// Oldest first: ord-2 (now-2s), ord-1 (now-1s), ord-0 (now).
	want := []string{"ord-2", "ord-1", "ord-0"}
	for i, rec := range records {
		if rec.ID != want[i] {
			t.Errorf("records[%d].ID = %q, want %q", i, rec.ID, want[i])
		}
	}
}

func TestPeek_FiltersOnNextRetryAt(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	// Enqueue a record, then push its next_retry_at into the future via RetryLater.
	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "future-1",
		UserID:  "user-1",
		CostUSD: 0.01,
		Tokens:  10,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// RetryLater with attempt=5 → backoff ~320s, well into the future.
	if err := ob.RetryLater(ctx, "future-1", 5); err != nil {
		t.Fatalf("RetryLater: %v", err)
	}

	// Peek should return nothing — next_retry_at is in the future.
	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0 (next_retry_at is in the future)", len(records))
	}

	// The record should still be pending.
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 1 {
		t.Errorf("pending = %d, want 1", count)
	}
}

func TestRetryLater_BackoffDelay(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	if err := ob.Enqueue(ctx, SpendRecord{
		ID:      "backoff-1",
		UserID:  "user-1",
		CostUSD: 0.01,
		Tokens:  10,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// RetryLater increments attempt_count and schedules future retry.
	if err := ob.RetryLater(ctx, "backoff-1", 0); err != nil {
		t.Fatalf("RetryLater: %v", err)
	}

	// Peek should return nothing — the next retry is 10s in the future.
	records, err := ob.Peek(ctx, 10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0 after RetryLater", len(records))
	}

	// But the record should still be pending with attempt_count incremented.
	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 1 {
		t.Errorf("pending = %d, want 1", count)
	}
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 10 * time.Second},
		{1, 20 * time.Second},
		{2, 40 * time.Second},
		{3, 80 * time.Second},
		{4, 160 * time.Second},
		{10, time.Hour}, // capped at 1 hour
		{20, time.Hour}, // still capped
	}
	for _, tt := range tests {
		got := backoffDelay(tt.attempt)
		if got != tt.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestDelete_Chunking(t *testing.T) {
	ob := newTestOutbox(t)
	ctx := context.Background()

	// Insert 600 records to test chunking (>500).
	const total = 600
	ids := make([]string, total)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("chunk-%04d", i)
		ids[i] = id
		if err := ob.Enqueue(ctx, SpendRecord{
			ID:      id,
			UserID:  "u",
			CostUSD: 0.001,
			Tokens:  1,
		}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	count, err := ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != total {
		t.Fatalf("inserted %d, want %d", count, total)
	}

	// Delete all — this should chunk into 500 + 100.
	if err := ob.Delete(ctx, ids); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	count, err = ob.Pending(ctx)
	if err != nil {
		t.Fatalf("Pending after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("remaining = %d, want 0", count)
	}
}
