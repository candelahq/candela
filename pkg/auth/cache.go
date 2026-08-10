package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// IdentityCache provides an in-memory LRU cache for resolved identities.
// Token-to-identity resolution (especially OAuth2 userinfo calls) can add
// 50-100ms per request. Caching reduces this to near-zero for repeat tokens.
//
// Security: cache keys are SHA-256 hashes of tokens, not raw tokens.
// This prevents credential leakage in heap dumps or debug output.
type IdentityCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	maxSize int
	ttl     time.Duration
	now     func() time.Time // injectable for testing
}

type cacheEntry struct {
	identity  *Identity
	expiresAt time.Time
}

// NewIdentityCache creates a cache with the given maximum size and TTL.
// Typical values: maxSize=1000, ttl=120s.
func NewIdentityCache(maxSize int, ttl time.Duration) *IdentityCache {
	return &IdentityCache{
		entries: make(map[string]cacheEntry, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
		now:     time.Now,
	}
}

// Get retrieves a cached identity by token. Returns (nil, false) on miss or expiry.
func (c *IdentityCache) Get(token string) (*Identity, bool) {
	key := tokenHash(token)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if c.now().After(entry.expiresAt) {
		// Expired — remove lazily
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.identity, true
}

// Put stores a resolved identity in the cache. If the cache is full,
// expired entries are evicted first. If still full, the oldest entry
// is evicted (simple eviction — not full LRU, but sufficient for auth tokens
// where the working set is small).
func (c *IdentityCache) Put(token string, id *Identity) {
	key := tokenHash(token)
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries if at capacity
	if len(c.entries) >= c.maxSize {
		now := c.now()
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
	}

	// If still at capacity, evict the oldest entry
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		delete(c.entries, oldestKey)
	}

	c.entries[key] = cacheEntry{
		identity:  id,
		expiresAt: c.now().Add(c.ttl),
	}
}

// Invalidate removes a specific token from the cache.
func (c *IdentityCache) Invalidate(token string) {
	key := tokenHash(token)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len returns the number of entries currently in the cache (including expired).
func (c *IdentityCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ResetForTest clears the cache and optionally injects a custom time source.
// Only use in tests.
func (c *IdentityCache) ResetForTest(nowFn func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry, c.maxSize)
	if nowFn != nil {
		c.now = nowFn
	}
}

// tokenHash returns the SHA-256 hex digest of a token string.
// Used as cache key to avoid storing raw credentials in memory.
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
