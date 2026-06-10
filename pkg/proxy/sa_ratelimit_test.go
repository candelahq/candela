package proxy

import (
	"fmt"
	"sync"
	"testing"
)

func TestSARateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := newSARateLimiter(5) // 5 RPM
	sa := "ci-bot@project.iam.gserviceaccount.com"

	for i := 1; i <= 5; i++ {
		allowed, count, limit := rl.Allow(sa)
		if !allowed {
			t.Errorf("request %d: expected allowed, got blocked", i)
		}
		if count != i {
			t.Errorf("request %d: count = %d, want %d", i, count, i)
		}
		if limit != 5 {
			t.Errorf("request %d: limit = %d, want 5", i, limit)
		}
	}

	// 6th request should be blocked.
	allowed, count, _ := rl.Allow(sa)
	if allowed {
		t.Errorf("request 6: expected blocked, got allowed (count=%d)", count)
	}
	if count != 6 {
		t.Errorf("request 6: count = %d, want 6", count)
	}
}

func TestSARateLimiter_IndependentPerSA(t *testing.T) {
	rl := newSARateLimiter(3)
	sa1 := "bot-a@project.iam.gserviceaccount.com"
	sa2 := "bot-b@project.iam.gserviceaccount.com"

	// Exhaust sa1's limit.
	for i := 0; i < 3; i++ {
		rl.Allow(sa1)
	}
	allowed, _, _ := rl.Allow(sa1)
	if allowed {
		t.Error("sa1 should be blocked after 3 requests")
	}

	// sa2 should still be allowed.
	allowed, count, _ := rl.Allow(sa2)
	if !allowed {
		t.Error("sa2 should be allowed — independent counter")
	}
	if count != 1 {
		t.Errorf("sa2 count = %d, want 1", count)
	}
}

func TestSARateLimiter_NewWindowResets(t *testing.T) {
	rl := newSARateLimiter(2)
	sa := "bot@project.iam.gserviceaccount.com"

	// Fill the window.
	rl.Allow(sa)
	rl.Allow(sa)
	allowed, _, _ := rl.Allow(sa)
	if allowed {
		t.Error("should be blocked at limit 2")
	}

	// Simulate a new minute by directly updating the window.
	rl.mu.Lock()
	rl.windows[sa].minute = "1970-01-01T00:00" // old minute
	rl.mu.Unlock()

	// Next request should start a new window.
	allowed, count, _ := rl.Allow(sa)
	if !allowed {
		t.Error("new minute window should allow requests")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (fresh window)", count)
	}
}

func TestSARateLimiter_DefaultLimit(t *testing.T) {
	rl := newSARateLimiter(0)
	if rl.limit != defaultSARateLimit {
		t.Errorf("limit = %d, want %d", rl.limit, defaultSARateLimit)
	}
}

func TestSARateLimiter_GC(t *testing.T) {
	rl := newSARateLimiter(100)
	rl.gcEvery = 5 // GC every 5 calls

	// Add some SAs.
	for i := 0; i < 3; i++ {
		rl.Allow(fmt.Sprintf("sa-%d@project.iam.gserviceaccount.com", i))
	}

	// Expire them by setting old minute.
	rl.mu.Lock()
	for _, w := range rl.windows {
		w.minute = "2020-01-01T00:00"
	}
	rl.mu.Unlock()

	// Trigger GC by calling Allow enough times.
	for i := 0; i < 5; i++ {
		rl.Allow("trigger@project.iam.gserviceaccount.com")
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// Only the trigger SA should remain.
	if len(rl.windows) != 1 {
		t.Errorf("after GC: %d windows remain, want 1", len(rl.windows))
	}
}

func TestSARateLimiter_Concurrent(t *testing.T) {
	rl := newSARateLimiter(100)
	sa := "concurrent@project.iam.gserviceaccount.com"

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow(sa)
		}()
	}
	wg.Wait()

	// Exactly 100 should have been allowed.
	rl.mu.Lock()
	count := rl.windows[sa].count
	rl.mu.Unlock()
	if count != 200 {
		t.Errorf("total count = %d, want 200 (all 200 hit the counter)", count)
	}
}
