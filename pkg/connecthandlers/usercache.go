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
// The cache evicts expired entries on every write to bound memory in long-running
// instances, keeping the map size proportional to the number of active users.
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
	userIDCacheNegativeTTL = 5 * time.Second // short TTL for not-found entries
)

var globalUserIDCache = &userIDCache{
	entries: make(map[string]userIDEntry),
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

	// Cache the result and evict expired entries to bound map growth.
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries[key] = entry
	// Evict expired entries on every write — keeps map size proportional
	// to the number of active users, not total unique users ever seen.
	for k, e := range globalUserIDCache.entries {
		if now.After(e.expiresAt) {
			delete(globalUserIDCache.entries, k)
		}
	}
	globalUserIDCache.mu.Unlock()

	return entry.userID, nil
}

// resetUserIDCache clears the global cache. Used in tests to prevent pollution.
func resetUserIDCache() {
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries = make(map[string]userIDEntry)
	globalUserIDCache.mu.Unlock()
}
