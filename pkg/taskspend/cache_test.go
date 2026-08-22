package taskspend

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
)

type mockStore struct {
	budget  *billing.TaskBudget
	err     error
	fetches atomic.Int64
}

func (m *mockStore) GetTaskBudget(_ context.Context, taskID string) (*billing.TaskBudget, error) {
	m.fetches.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	if m.budget == nil {
		return nil, storage.ErrNotFound
	}
	// Return a copy to avoid mutation.
	b := *m.budget
	return &b, nil
}

func TestCache_HitAndMiss(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-1",
			LimitUSD: 10.0,
			SpentUSD: 3.5,
		},
	}
	c := New(store, 1*time.Second)

	// First call: cache miss → fetches from store.
	snap, err := c.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.TaskID != "job-1" {
		t.Errorf("TaskID = %q, want %q", snap.TaskID, "job-1")
	}
	if snap.RemainingUSD != 6.5 {
		t.Errorf("RemainingUSD = %v, want 6.5", snap.RemainingUSD)
	}
	if store.fetches.Load() != 1 {
		t.Errorf("fetches = %d, want 1", store.fetches.Load())
	}

	// Second call: cache hit → no additional fetch.
	_, err = c.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("Get (cached): %v", err)
	}
	if store.fetches.Load() != 1 {
		t.Errorf("fetches = %d, want 1 (should be cached)", store.fetches.Load())
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-2",
			LimitUSD: 5.0,
			SpentUSD: 1.0,
		},
	}
	c := New(store, 50*time.Millisecond)

	// Prime cache.
	if _, err := c.Get(context.Background(), "job-2"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	// Update store data.
	store.budget.SpentUSD = 4.0

	// Should re-fetch.
	snap, err := c.Get(context.Background(), "job-2")
	if err != nil {
		t.Fatalf("Get (after TTL): %v", err)
	}
	if store.fetches.Load() != 2 {
		t.Errorf("fetches = %d, want 2", store.fetches.Load())
	}
	if snap.SpentUSD != 4.0 {
		t.Errorf("SpentUSD = %v, want 4.0", snap.SpentUSD)
	}
}

func TestCache_NotFound(t *testing.T) {
	store := &mockStore{err: storage.ErrNotFound}
	c := New(store, 1*time.Second)

	_, err := c.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestCache_StoreError(t *testing.T) {
	store := &mockStore{err: fmt.Errorf("firestore unavailable")}
	c := New(store, 1*time.Second)

	_, err := c.Get(context.Background(), "job-err")
	if err == nil {
		t.Fatal("expected error on store failure")
	}
}

func TestCache_EmptyTaskID(t *testing.T) {
	c := New(&mockStore{}, 1*time.Second)
	_, err := c.Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestCache_Invalidate(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-inv",
			LimitUSD: 10.0,
		},
	}
	c := New(store, 10*time.Second)

	// Prime.
	if _, err := c.Get(context.Background(), "job-inv"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Invalidate and re-fetch.
	c.Invalidate("job-inv")
	if _, err := c.Get(context.Background(), "job-inv"); err != nil {
		t.Fatalf("Get (after invalidate): %v", err)
	}
	if store.fetches.Load() != 2 {
		t.Errorf("fetches = %d, want 2", store.fetches.Load())
	}
}

func TestCache_Evict(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-evict",
			LimitUSD: 5.0,
		},
	}
	c := New(store, 50*time.Millisecond)

	// Prime.
	if _, err := c.Get(context.Background(), "job-evict"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}

	// Wait for TTL.
	time.Sleep(60 * time.Millisecond)
	c.Evict()
	if c.Len() != 0 {
		t.Errorf("Len after Evict = %d, want 0", c.Len())
	}
}

func TestCache_ExpiredBudget(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:    "job-exp",
			LimitUSD:  10.0,
			SpentUSD:  2.0,
			ExpiresAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	c := New(store, 1*time.Second)

	snap, err := c.Get(context.Background(), "job-exp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !snap.Expired {
		t.Error("expected Expired=true for expired budget")
	}
}
