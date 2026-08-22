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

func TestCache_DefaultTTL(t *testing.T) {
	c := New(&mockStore{}, 0)
	if c.ttl != DefaultTTL {
		t.Errorf("ttl = %v, want %v", c.ttl, DefaultTTL)
	}
}

func TestCache_SingleflightColdMiss(t *testing.T) {
	// Concurrent requests for the same cold task should produce exactly 1 fetch.
	store := &slowStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-sf",
			LimitUSD: 10.0,
			SpentUSD: 1.0,
		},
		delay: 50 * time.Millisecond,
	}
	c := New(store, 5*time.Second)

	const n = 10
	errs := make(chan error, n)
	snaps := make(chan *SpendSnapshot, n)

	for i := 0; i < n; i++ {
		go func() {
			snap, err := c.Get(context.Background(), "job-sf")
			errs <- err
			snaps <- snap
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		snap := <-snaps
		if snap.TaskID != "job-sf" {
			t.Errorf("goroutine %d: TaskID = %q, want %q", i, snap.TaskID, "job-sf")
		}
	}

	fetches := store.fetches.Load()
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1 (singleflight should coalesce)", fetches)
	}
}

func TestCache_SingleflightStaleRefresh(t *testing.T) {
	// Concurrent requests for the same stale entry should coalesce into 1 re-fetch.
	store := &slowStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-stale",
			LimitUSD: 10.0,
			SpentUSD: 2.0,
		},
		delay: 50 * time.Millisecond,
	}
	c := New(store, 30*time.Millisecond)

	// Prime cache.
	if _, err := c.Get(context.Background(), "job-stale"); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if store.fetches.Load() != 1 {
		t.Fatalf("priming fetches = %d, want 1", store.fetches.Load())
	}

	// Wait for TTL to expire.
	time.Sleep(40 * time.Millisecond)

	// Fire concurrent refreshes.
	const n = 10
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := c.Get(context.Background(), "job-stale")
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	fetches := store.fetches.Load()
	if fetches != 2 {
		t.Errorf("fetches = %d, want 2 (1 prime + 1 coalesced refresh)", fetches)
	}
}

func TestCache_SingleflightErrorPropagation(t *testing.T) {
	// All waiters should see the same error.
	store := &slowStore{
		err:   fmt.Errorf("boom"),
		delay: 50 * time.Millisecond,
	}
	c := New(store, 5*time.Second)

	const n = 5
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := c.Get(context.Background(), "job-err-sf")
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err == nil {
			t.Errorf("goroutine %d: expected error, got nil", i)
		}
	}
}

// slowStore adds a configurable delay to simulate Firestore latency
// for singleflight tests.
type slowStore struct {
	budget  *billing.TaskBudget
	err     error
	delay   time.Duration
	fetches atomic.Int64
}

func (s *slowStore) GetTaskBudget(_ context.Context, _ string) (*billing.TaskBudget, error) {
	s.fetches.Add(1)
	time.Sleep(s.delay)
	if s.err != nil {
		return nil, s.err
	}
	b := *s.budget
	return &b, nil
}
