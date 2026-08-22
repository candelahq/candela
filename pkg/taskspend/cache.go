// Package taskspend provides an in-memory cache for task budget spend
// with a short TTL and Firestore read-through. This avoids hitting
// Firestore on every status poll while keeping the data reasonably fresh.
package taskspend

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/billing"
)

// DefaultTTL is the default cache entry lifetime.
const DefaultTTL = 5 * time.Second

// Store is the minimal interface for reading task budgets.
type Store interface {
	GetTaskBudget(ctx context.Context, taskID string) (*billing.TaskBudget, error)
}

// entry is a single cached task budget snapshot.
type entry struct {
	budget    *billing.TaskBudget
	fetchedAt time.Time
}

// Cache is an in-memory cache for task budget spend data.
// It provides read-through to a backing Store with a configurable TTL.
//
// Safe for concurrent use.
type Cache struct {
	store Store
	ttl   time.Duration

	mu      sync.RWMutex
	entries map[string]*entry
}

// New creates a TaskSpendCache backed by the given store.
// If ttl is zero, DefaultTTL is used.
func New(store Store, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Cache{
		store:   store,
		ttl:     ttl,
		entries: make(map[string]*entry),
	}
}

// SpendSnapshot is the public read-only view returned by Get.
type SpendSnapshot struct {
	TaskID       string  `json:"task_id"`
	LimitUSD     float64 `json:"limit_usd"`
	SpentUSD     float64 `json:"spent_usd"`
	RemainingUSD float64 `json:"remaining_usd"`
	Expired      bool    `json:"expired"`
	CachedAt     string  `json:"cached_at"`
}

// Get returns the cached spend snapshot for a task, fetching from
// the store if the cache is cold or stale.
func (c *Cache) Get(ctx context.Context, taskID string) (*SpendSnapshot, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskspend: task_id is required")
	}

	// Fast path: check cache under read lock.
	c.mu.RLock()
	if e, ok := c.entries[taskID]; ok && time.Since(e.fetchedAt) < c.ttl {
		snap := toSnapshot(e)
		c.mu.RUnlock()
		return snap, nil
	}
	c.mu.RUnlock()

	// Slow path: fetch from store.
	budget, err := c.store.GetTaskBudget(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("taskspend: fetching task budget: %w", err)
	}

	now := time.Now().UTC()
	e := &entry{
		budget:    budget,
		fetchedAt: now,
	}

	c.mu.Lock()
	c.entries[taskID] = e
	c.mu.Unlock()

	return toSnapshot(e), nil
}

// Invalidate removes a task's cached entry, forcing the next Get to
// read through to the store.
func (c *Cache) Invalidate(taskID string) {
	c.mu.Lock()
	delete(c.entries, taskID)
	c.mu.Unlock()
}

// Evict removes all expired cache entries. Call periodically from a
// background goroutine to prevent unbounded memory growth.
func (c *Cache) Evict() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, e := range c.entries {
		if now.Sub(e.fetchedAt) >= c.ttl {
			delete(c.entries, id)
		}
	}
}

// StartEvictor launches a background goroutine that calls Evict every
// interval. It stops when the context is cancelled. Returns immediately.
func (c *Cache) StartEvictor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Evict()
			}
		}
	}()
	slog.Info("taskspend: cache evictor started", "interval", interval, "ttl", c.ttl)
}

// Len returns the current number of cached entries (for testing/metrics).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func toSnapshot(e *entry) *SpendSnapshot {
	b := e.budget
	return &SpendSnapshot{
		TaskID:       b.TaskID,
		LimitUSD:     b.LimitUSD,
		SpentUSD:     b.SpentUSD,
		RemainingUSD: b.Remaining(),
		Expired:      b.IsExpired(),
		CachedAt:     e.fetchedAt.Format(time.RFC3339Nano),
	}
}
