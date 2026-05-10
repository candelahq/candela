package connecthandlers

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// userIDCache is an in-process cache from email → Firestore user ID.
//
// Every handler call (GetMyUsage, GetMyBudget, GetTeamLeaderboard, …) resolves
// the caller's email to a Firestore doc ID via GetUserByEmail. For a user that
// calls the dashboard every 5 seconds, that's 12 Firestore reads per minute
// just for identity resolution. This cache eliminates the redundant reads.
//
// TTL is intentionally short (60s) so:
//   - Provisioning delays resolve quickly after a new user is created.
//   - Email changes (rare) propagate within a minute.
//
// Keys are lowercased so different casings for the same address share a slot.
//
// Eviction: a background goroutine sweeps expired entries every 30 seconds so
// that writes never hold the write lock while scanning the whole map (O(N)).
// This keeps write-lock critical sections O(1) regardless of cache size.
//
// #B: Negative entries (user not found) are cached with a 5-second TTL to
// prevent a thundering herd of Firestore reads when a user is not yet provisioned.
type userIDCache struct {
	mu      sync.RWMutex
	entries map[string]userIDEntry
}

type userIDEntry struct {
	userID    string // empty string = negative cache (user not found)
	found     bool   // false = negative cache entry
	expiresAt time.Time
}

const (
	userIDCacheTTL         = 60 * time.Second
	userIDCacheNegativeTTL = 5 * time.Second  // short TTL for not-found entries
	userIDCacheSweepPeriod = 30 * time.Second // background eviction cadence
)

var globalUserIDCache = func() *userIDCache {
	c := &userIDCache{
		entries: make(map[string]userIDEntry),
	}
	// Background goroutine: sweep expired entries every 30 seconds.
	// This keeps write-path critical sections O(1) — no per-write map scan.
	go func() {
		ticker := time.NewTicker(userIDCacheSweepPeriod)
		defer ticker.Stop()
		for range ticker.C {
			c.evictExpired()
		}
	}()
	return c
}()

// evictExpired removes all entries whose TTL has elapsed. Called by the
// background sweep goroutine; must not be called while already holding the lock.
func (c *userIDCache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

// resolveUserID returns the Firestore user ID for the given email, using a
// 60-second in-process cache. Returns ("", nil) if the user doesn't exist yet.
func resolveUserID(ctx context.Context, users storage.UserStore, email string) (string, error) {
	if users == nil {
		return "", nil
	}

	// Normalize key: emails are case-insensitive; different casings must share a slot.
	key := strings.ToLower(email)

	// Fast path: cache hit (positive or negative).
	globalUserIDCache.mu.RLock()
	if e, ok := globalUserIDCache.entries[key]; ok && time.Now().Before(e.expiresAt) {
		globalUserIDCache.mu.RUnlock()
		return e.userID, nil // empty string on negative cache — caller treats "" as "skip"
	}
	globalUserIDCache.mu.RUnlock()

	// Slow path: Firestore lookup.
	user, err := users.GetUserByEmail(ctx, email)
	if err != nil {
		// Don't cache errors — they may be transient (network, Firestore blip).
		return "", err
	}

	now := time.Now()
	var entry userIDEntry

	if user == nil {
		// #B: Negative cache — user not found yet (still provisioning).
		// Short TTL so the cache clears quickly once the user is created.
		entry = userIDEntry{userID: "", found: false, expiresAt: now.Add(userIDCacheNegativeTTL)}
	} else {
		entry = userIDEntry{userID: user.ID, found: true, expiresAt: now.Add(userIDCacheTTL)}
	}

	// Store result. The background goroutine handles eviction — no per-write scan.
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries[key] = entry
	globalUserIDCache.mu.Unlock()

	return entry.userID, nil
}

// resetUserIDCache clears the global cache. Used in tests to prevent pollution.
func resetUserIDCache() {
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries = make(map[string]userIDEntry)
	globalUserIDCache.mu.Unlock()
}
