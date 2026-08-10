package auth

import (
	"sync"
	"testing"
	"time"
)

func TestCache_PutAndGet(t *testing.T) {
	c := NewIdentityCache(10, 60*time.Second)
	id := &Identity{ID: "uid", Email: "a@b.com", Provider: "firebase"}

	c.Put("token-abc", id)

	got, ok := c.Get("token-abc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Email != "a@b.com" {
		t.Errorf("Email = %q, want %q", got.Email, "a@b.com")
	}
	if got.Provider != "firebase" {
		t.Errorf("Provider = %q, want %q", got.Provider, "firebase")
	}
}

func TestCache_Miss(t *testing.T) {
	c := NewIdentityCache(10, 60*time.Second)

	_, ok := c.Get("nonexistent-token")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewIdentityCache(10, 30*time.Second)
	c.now = func() time.Time { return now }

	c.Put("token-1", &Identity{ID: "uid", Email: "a@b.com"})

	// Still valid at +29s
	c.now = func() time.Time { return now.Add(29 * time.Second) }
	if _, ok := c.Get("token-1"); !ok {
		t.Fatal("expected cache hit at 29s")
	}

	// Expired at +31s
	c.now = func() time.Time { return now.Add(31 * time.Second) }
	if _, ok := c.Get("token-1"); ok {
		t.Fatal("expected cache miss at 31s (expired)")
	}

	// Entry should be removed
	if c.Len() != 0 {
		t.Errorf("Len() = %d after expiry, want 0", c.Len())
	}
}

func TestCache_MaxSizeEviction(t *testing.T) {
	c := NewIdentityCache(3, 60*time.Second)

	c.Put("t1", &Identity{ID: "1", Email: "1@b.com"})
	c.Put("t2", &Identity{ID: "2", Email: "2@b.com"})
	c.Put("t3", &Identity{ID: "3", Email: "3@b.com"})

	if c.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", c.Len())
	}

	// Adding a 4th should evict one
	c.Put("t4", &Identity{ID: "4", Email: "4@b.com"})

	if c.Len() != 3 {
		t.Errorf("Len() = %d after overflow, want 3", c.Len())
	}

	// t4 should be present
	if _, ok := c.Get("t4"); !ok {
		t.Error("expected t4 to be cached")
	}
}

func TestCache_EvictsExpiredFirst(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewIdentityCache(2, 10*time.Second)
	c.now = func() time.Time { return now }

	c.Put("old", &Identity{ID: "old"})

	// Advance time so "old" is expired
	c.now = func() time.Time { return now.Add(15 * time.Second) }

	c.Put("new1", &Identity{ID: "new1"})
	c.Put("new2", &Identity{ID: "new2"})

	// "old" was expired and should have been evicted
	if _, ok := c.Get("old"); ok {
		t.Error("expected expired entry to be evicted")
	}
	if _, ok := c.Get("new1"); !ok {
		t.Error("expected new1 to be present")
	}
	if _, ok := c.Get("new2"); !ok {
		t.Error("expected new2 to be present")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := NewIdentityCache(10, 60*time.Second)
	c.Put("token-1", &Identity{ID: "uid"})

	c.Invalidate("token-1")

	if _, ok := c.Get("token-1"); ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestCache_HashesTokens(t *testing.T) {
	c := NewIdentityCache(10, 60*time.Second)
	c.Put("secret-token-value", &Identity{ID: "uid"})

	// Verify raw token is not stored as a key
	c.mu.RLock()
	defer c.mu.RUnlock()
	for key := range c.entries {
		if key == "secret-token-value" {
			t.Fatal("raw token stored as cache key — security risk")
		}
		// SHA-256 hex is always 64 chars
		if len(key) != 64 {
			t.Errorf("key length = %d, want 64 (SHA-256 hex)", len(key))
		}
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewIdentityCache(100, 60*time.Second)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			token := "token-" + string(rune('A'+n))
			c.Put(token, &Identity{ID: token})
		}(i)
	}

	// Concurrent reads
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get("token-A")
		}()
	}

	wg.Wait()
	// No panic = pass
}

func TestCache_ResetForTest(t *testing.T) {
	c := NewIdentityCache(10, 60*time.Second)
	c.Put("t1", &Identity{ID: "1"})

	customNow := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	c.ResetForTest(func() time.Time { return customNow })

	if c.Len() != 0 {
		t.Errorf("Len() = %d after reset, want 0", c.Len())
	}

	// Verify custom time is used
	c.Put("t2", &Identity{ID: "2"})
	c.mu.RLock()
	for _, entry := range c.entries {
		if !entry.expiresAt.After(customNow) {
			t.Error("expected custom time source to be used")
		}
	}
	c.mu.RUnlock()
}

func TestCache_ZeroMaxSize(t *testing.T) {
	c := NewIdentityCache(0, 60*time.Second)
	c.Put("token-1", &Identity{ID: "uid"})

	if c.Len() != 0 {
		t.Errorf("Len() = %d for zero-size cache, want 0", c.Len())
	}
	if _, ok := c.Get("token-1"); ok {
		t.Error("expected miss on zero-size cache")
	}
}

func TestCache_NegativeMaxSize(t *testing.T) {
	c := NewIdentityCache(-5, 60*time.Second)
	c.Put("token-1", &Identity{ID: "uid"})

	if c.Len() != 0 {
		t.Errorf("Len() = %d for negative-size cache, want 0", c.Len())
	}
}

func TestCache_UpdateExistingKeyNoEviction(t *testing.T) {
	c := NewIdentityCache(2, 60*time.Second)

	c.Put("t1", &Identity{ID: "1", Email: "1@b.com"})
	c.Put("t2", &Identity{ID: "2", Email: "2@b.com"})

	// Update t1 — should NOT evict t2
	c.Put("t1", &Identity{ID: "1-updated", Email: "1-new@b.com"})

	if c.Len() != 2 {
		t.Fatalf("Len() = %d after update, want 2", c.Len())
	}

	got, ok := c.Get("t1")
	if !ok {
		t.Fatal("expected cache hit for t1")
	}
	if got.ID != "1-updated" {
		t.Errorf("ID = %q, want %q", got.ID, "1-updated")
	}

	// t2 must still be present
	if _, ok := c.Get("t2"); !ok {
		t.Error("expected t2 to still be cached after updating t1")
	}
}
