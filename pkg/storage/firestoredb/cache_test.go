package firestoredb

import (
	"testing"
	"time"
)

func TestRateLimitCache_MissReturnsZeroFalse(t *testing.T) {
	evictRateLimitCache("nonexistent@example.com")
	limit, ok := getCachedRateLimit("nonexistent@example.com")
	if ok {
		t.Errorf("expected cache miss, got limit=%d", limit)
	}
}

func TestRateLimitCache_SetThenHit(t *testing.T) {
	key := "cache-hit-test@example.com"
	evictRateLimitCache(key)
	setCachedRateLimit(key, 99)

	limit, ok := getCachedRateLimit(key)
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if limit != 99 {
		t.Errorf("limit = %d, want 99", limit)
	}
	evictRateLimitCache(key)
}

func TestRateLimitCache_Evict(t *testing.T) {
	key := "cache-evict-test@example.com"
	setCachedRateLimit(key, 42)
	evictRateLimitCache(key)

	_, ok := getCachedRateLimit(key)
	if ok {
		t.Error("expected cache miss after eviction, got hit")
	}
}

func TestRateLimitCache_ExpiredEntryIsMiss(t *testing.T) {
	key := "cache-expired-test@example.com"
	// Manually insert an already-expired entry.
	rateLimitValueCache.mu.Lock()
	rateLimitValueCache.entries[key] = rlCacheEntry{
		limit:     77,
		expiresAt: time.Now().Add(-1 * time.Second), // expired
	}
	rateLimitValueCache.mu.Unlock()

	_, ok := getCachedRateLimit(key)
	if ok {
		t.Error("expected cache miss for expired entry, got hit")
	}

	evictRateLimitCache(key) // cleanup
}

func TestSweepRateLimitCache_RemovesExpiredOnly(t *testing.T) {
	live := "cache-live@example.com"
	dead := "cache-dead@example.com"

	setCachedRateLimit(live, 10)
	rateLimitValueCache.mu.Lock()
	rateLimitValueCache.entries[dead] = rlCacheEntry{
		limit:     20,
		expiresAt: time.Now().Add(-1 * time.Minute), // expired
	}
	rateLimitValueCache.mu.Unlock()

	sweepRateLimitCache()

	if _, ok := getCachedRateLimit(live); !ok {
		t.Error("sweep removed live entry")
	}
	if _, ok := getCachedRateLimit(dead); ok {
		t.Error("sweep kept expired entry")
	}

	evictRateLimitCache(live)
}

func TestRateLimitCache_OverwriteUpdatesExpiry(t *testing.T) {
	key := "cache-overwrite@example.com"
	setCachedRateLimit(key, 5)
	setCachedRateLimit(key, 10) // overwrite

	limit, ok := getCachedRateLimit(key)
	if !ok || limit != 10 {
		t.Errorf("overwrite: got limit=%d ok=%v, want limit=10 ok=true", limit, ok)
	}
	evictRateLimitCache(key)
}
