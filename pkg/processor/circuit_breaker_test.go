package processor

import (
	"sync"
	"testing"
	"time"
)

// mockClock allows deterministic time control without sleeps.
type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock() *mockClock {
	return &mockClock{now: time.Now()}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestBreakerStateString(t *testing.T) {
	tests := []struct {
		state BreakerState
		want  string
	}{
		{BreakerClosed, "closed"},
		{BreakerOpen, "open"},
		{BreakerHalfOpen, "half-open"},
		{BreakerState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("BreakerState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestBreakerStartsClosed(t *testing.T) {
	b := NewBreaker(DefaultBreakerConfig(), newMockClock())
	if b.State() != BreakerClosed {
		t.Errorf("new breaker state = %v, want Closed", b.State())
	}
	if !b.Allow() {
		t.Error("new breaker should allow writes")
	}
}

func TestBreakerClosedToOpen(t *testing.T) {
	cfg := BreakerConfig{Threshold: 3, ResetTimeout: 10 * time.Second, HalfOpenMax: 1}
	b := NewBreaker(cfg, newMockClock())

	// Failures below threshold keep it closed.
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != BreakerClosed {
		t.Fatalf("after 2 failures (threshold=3): state = %v, want Closed", b.State())
	}
	if !b.Allow() {
		t.Fatal("should still allow writes below threshold")
	}

	// Third failure trips it.
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatalf("after 3 failures (threshold=3): state = %v, want Open", b.State())
	}
	if b.Allow() {
		t.Fatal("open breaker should not allow writes")
	}
}

func TestBreakerOpenToHalfOpen(t *testing.T) {
	clk := newMockClock()
	cfg := BreakerConfig{Threshold: 2, ResetTimeout: 5 * time.Second, HalfOpenMax: 1}
	b := NewBreaker(cfg, clk)

	// Trip it.
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want Open", b.State())
	}

	// Before timeout: still blocked.
	clk.Advance(4 * time.Second)
	if b.Allow() {
		t.Fatal("should not allow before reset timeout")
	}

	// After timeout: transitions to half-open and allows a probe.
	clk.Advance(2 * time.Second) // total 6s > 5s timeout
	if !b.Allow() {
		t.Fatal("should allow probe after reset timeout")
	}
	if b.State() != BreakerHalfOpen {
		t.Fatalf("state = %v, want HalfOpen", b.State())
	}
}

func TestBreakerHalfOpenToClosedOnSuccess(t *testing.T) {
	clk := newMockClock()
	cfg := BreakerConfig{Threshold: 1, ResetTimeout: 1 * time.Second, HalfOpenMax: 2}
	b := NewBreaker(cfg, clk)

	// Trip → open → half-open.
	b.RecordFailure()
	clk.Advance(2 * time.Second)
	b.Allow() // triggers half-open

	// First success: still half-open.
	b.RecordSuccess()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("after 1 success (halfOpenMax=2): state = %v, want HalfOpen", b.State())
	}

	// Second success: re-closes.
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatalf("after 2 successes: state = %v, want Closed", b.State())
	}
}

func TestBreakerHalfOpenToOpenOnFailure(t *testing.T) {
	clk := newMockClock()
	cfg := BreakerConfig{Threshold: 1, ResetTimeout: 1 * time.Second, HalfOpenMax: 2}
	b := NewBreaker(cfg, clk)

	// Trip → open → half-open.
	b.RecordFailure()
	clk.Advance(2 * time.Second)
	b.Allow()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("state = %v, want HalfOpen", b.State())
	}

	// Failure in half-open re-opens.
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want Open after half-open failure", b.State())
	}
}

func TestBreakerSuccessResetsClosed(t *testing.T) {
	cfg := BreakerConfig{Threshold: 3, ResetTimeout: 10 * time.Second, HalfOpenMax: 1}
	b := NewBreaker(cfg, newMockClock())

	// Accumulate some failures, then succeed.
	b.RecordFailure()
	b.RecordFailure()
	if b.ConsecutiveFailures() != 2 {
		t.Fatalf("failures = %d, want 2", b.ConsecutiveFailures())
	}

	b.RecordSuccess()
	if b.ConsecutiveFailures() != 0 {
		t.Fatalf("failures after success = %d, want 0", b.ConsecutiveFailures())
	}
	if b.State() != BreakerClosed {
		t.Fatalf("state = %v, want Closed", b.State())
	}
}

func TestBreakerFailureCountCaps(t *testing.T) {
	cfg := BreakerConfig{Threshold: 2, ResetTimeout: 10 * time.Second, HalfOpenMax: 1}
	b := NewBreaker(cfg, newMockClock())

	// Hammer failures well beyond threshold.
	for i := 0; i < 100; i++ {
		b.RecordFailure()
	}

	// Should be capped at threshold+1, not 100.
	if f := b.ConsecutiveFailures(); f > cfg.Threshold+1 {
		t.Errorf("failures = %d, want at most %d (capped)", f, cfg.Threshold+1)
	}
}

func TestBreakerDefaultConfig(t *testing.T) {
	// Zero-value config should get defaults applied.
	b := NewBreaker(BreakerConfig{}, nil)
	if b.threshold != 5 {
		t.Errorf("threshold = %d, want 5", b.threshold)
	}
	if b.resetTimeout != 30*time.Second {
		t.Errorf("resetTimeout = %v, want 30s", b.resetTimeout)
	}
	if b.halfOpenMax != 2 {
		t.Errorf("halfOpenMax = %d, want 2", b.halfOpenMax)
	}
	// Should use realClock when nil is passed.
	if !b.Allow() {
		t.Error("default breaker should allow writes")
	}
}

func TestBreakerConcurrency(t *testing.T) {
	clk := newMockClock()
	cfg := BreakerConfig{Threshold: 50, ResetTimeout: 1 * time.Second, HalfOpenMax: 5}
	b := NewBreaker(cfg, clk)

	var wg sync.WaitGroup
	// Hammer from multiple goroutines — no races should occur.
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			b.Allow()
		}()
		go func() {
			defer wg.Done()
			b.RecordFailure()
		}()
		go func() {
			defer wg.Done()
			b.RecordSuccess()
		}()
	}
	wg.Wait()

	// Just verify no panic/race — state is non-deterministic.
	_ = b.State()
	_ = b.ConsecutiveFailures()
}

func TestBreakerFullCycle(t *testing.T) {
	clk := newMockClock()
	cfg := BreakerConfig{Threshold: 2, ResetTimeout: 5 * time.Second, HalfOpenMax: 1}
	b := NewBreaker(cfg, clk)

	// 1. Closed → works fine.
	if !b.Allow() {
		t.Fatal("step 1: should allow")
	}

	// 2. Trip it.
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatal("step 2: should be open")
	}

	// 3. Wait for reset → half-open.
	clk.Advance(6 * time.Second)
	if !b.Allow() {
		t.Fatal("step 3: should allow probe")
	}
	if b.State() != BreakerHalfOpen {
		t.Fatal("step 3: should be half-open")
	}

	// 4. Probe fails → back to open.
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatal("step 4: should re-open")
	}

	// 5. Wait again → half-open → success → closed.
	clk.Advance(6 * time.Second)
	b.Allow()
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatal("step 5: should be closed after successful probe")
	}
}
