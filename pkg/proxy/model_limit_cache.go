// Package proxy — model_limit_cache.go provides a per-user model limit cache
// that loads limits from Firestore and merges them with global YAML defaults.
// Used by the proxy pre-flight gate to enforce per-user per-model daily spend
// limits (#721, PR 2).
//
// Cache behavior:
//   - 60s TTL with lazy expiry (checked on read, no background evictor)
//   - Per-Proxy-instance: in a multi-replica deployment, each replica has its
//     own cache. Admin changes propagate within ≤60s per replica.
//   - Fail-open: if Firestore is unreachable, falls back to YAML-only limits.
package proxy

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// modelLimitStore is the minimal interface for loading per-user model limits.
type modelLimitStore interface {
	GetModelLimits(ctx context.Context, userID string) ([]*storage.ModelLimitRecord, error)
}

// mlCacheEntry holds cached per-user model limits with a fetch timestamp.
type mlCacheEntry struct {
	limits    []SpendLimitConfig
	fetchedAt time.Time
}

// modelLimitCache is a TTL cache for per-user model limits loaded from Firestore.
// Thread-safe for concurrent reads/writes from proxy request handlers.
type modelLimitCache struct {
	mu      sync.RWMutex
	entries map[string]*mlCacheEntry
	ttl     time.Duration
	store   modelLimitStore
}

// newModelLimitCache creates a cache with the given TTL and backing store.
func newModelLimitCache(store modelLimitStore, ttl time.Duration) *modelLimitCache {
	return &modelLimitCache{
		entries: make(map[string]*mlCacheEntry),
		ttl:     ttl,
		store:   store,
	}
}

// getUserLimits returns the cached per-user limits, refreshing from the store
// if the entry is missing or expired. Returns nil on store errors (fail-open).
func (c *modelLimitCache) getUserLimits(ctx context.Context, userID string) []SpendLimitConfig {
	// Fast path: check cache under read lock.
	c.mu.RLock()
	entry, ok := c.entries[userID]
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		limits := entry.limits
		c.mu.RUnlock()
		return limits
	}
	c.mu.RUnlock()

	// Slow path: fetch from store under write lock.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed).
	entry, ok = c.entries[userID]
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return entry.limits
	}

	records, err := c.store.GetModelLimits(ctx, userID)
	if err != nil {
		slog.Warn("model limit cache: failed to load user limits, falling back to YAML",
			"user_id", userID, "error", err)
		// Fail-open: return nil so mergedLimits falls back to YAML-only.
		return nil
	}

	limits := recordsToLimits(records)
	c.entries[userID] = &mlCacheEntry{
		limits:    limits,
		fetchedAt: time.Now(),
	}
	return limits
}

// mergedLimits returns the effective limits for a user by merging their
// Firestore limits with the global YAML defaults. User limits override YAML
// for the same model prefix; YAML-only and user-only limits are both included.
func (c *modelLimitCache) mergedLimits(ctx context.Context, userID string, yamlLimits []SpendLimitConfig) []SpendLimitConfig {
	userLimits := c.getUserLimits(ctx, userID)
	if len(userLimits) == 0 {
		return yamlLimits
	}
	if len(yamlLimits) == 0 {
		return userLimits
	}

	// Build a set of user-overridden model prefixes (lowercased).
	userPrefixes := make(map[string]struct{}, len(userLimits))
	for _, ul := range userLimits {
		userPrefixes[strings.ToLower(ul.Model)] = struct{}{}
	}

	// Start with user limits, then append YAML limits that aren't overridden.
	merged := make([]SpendLimitConfig, 0, len(userLimits)+len(yamlLimits))
	merged = append(merged, userLimits...)
	for _, yl := range yamlLimits {
		if _, overridden := userPrefixes[strings.ToLower(yl.Model)]; !overridden {
			merged = append(merged, yl)
		}
	}
	return merged
}

// recordsToLimits converts storage records to SpendLimitConfig slice.
func recordsToLimits(records []*storage.ModelLimitRecord) []SpendLimitConfig {
	if len(records) == 0 {
		return nil
	}
	limits := make([]SpendLimitConfig, len(records))
	for i, r := range records {
		limits[i] = SpendLimitConfig{
			Model:       r.ModelPrefix,
			MaxDailyUSD: r.MaxDailyUSD,
		}
	}
	return limits
}
