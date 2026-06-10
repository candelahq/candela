package proxy

import (
	"sync"
	"testing"
)

func TestPendingSpend_ReserveAndRelease(t *testing.T) {
	ps := newPendingSpendTracker()
	user := "user@example.com"

	// Reserve $1.00
	total := ps.Reserve(user, 1.0)
	if total != 1.0 {
		t.Errorf("after reserve $1: total = %f, want 1.0", total)
	}

	// Reserve another $0.50
	total = ps.Reserve(user, 0.5)
	if total != 1.5 {
		t.Errorf("after reserve $0.50: total = %f, want 1.5", total)
	}

	// Get should return 1.5
	if got := ps.Get(user); got != 1.5 {
		t.Errorf("Get = %f, want 1.5", got)
	}

	// Release $1.00
	ps.Release(user, 1.0)
	if got := ps.Get(user); got != 0.5 {
		t.Errorf("after release $1: Get = %f, want 0.5", got)
	}

	// Release remaining
	ps.Release(user, 0.5)
	if got := ps.Get(user); got != 0 {
		t.Errorf("after full release: Get = %f, want 0", got)
	}
}

func TestPendingSpend_IndependentUsers(t *testing.T) {
	ps := newPendingSpendTracker()
	ps.Reserve("alice@ex.com", 5.0)
	ps.Reserve("bob@ex.com", 3.0)

	if got := ps.Get("alice@ex.com"); got != 5.0 {
		t.Errorf("alice = %f, want 5.0", got)
	}
	if got := ps.Get("bob@ex.com"); got != 3.0 {
		t.Errorf("bob = %f, want 3.0", got)
	}
}

func TestPendingSpend_ReleaseClamps(t *testing.T) {
	ps := newPendingSpendTracker()
	ps.Reserve("user@ex.com", 1.0)
	ps.Release("user@ex.com", 5.0) // Release more than reserved
	if got := ps.Get("user@ex.com"); got != 0 {
		t.Errorf("over-release: Get = %f, want 0", got)
	}
}

func TestPendingSpend_ZeroAndNegative(t *testing.T) {
	ps := newPendingSpendTracker()
	// Zero and negative amounts should be no-ops
	ps.Reserve("user@ex.com", 0)
	ps.Reserve("user@ex.com", -1.0)
	ps.Reserve("", 5.0)
	if got := ps.Get("user@ex.com"); got != 0 {
		t.Errorf("zero/negative reserve: Get = %f, want 0", got)
	}
}

func TestPendingSpend_Concurrent(t *testing.T) {
	ps := newPendingSpendTracker()
	user := "concurrent@ex.com"

	var wg sync.WaitGroup
	// 100 goroutines each reserve $0.01
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ps.Reserve(user, 0.01)
		}()
	}
	wg.Wait()

	got := ps.Get(user)
	// Allow for float64 imprecision
	if got < 0.99 || got > 1.01 {
		t.Errorf("concurrent reserve: Get = %f, want ~1.0", got)
	}

	// Now release all
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ps.Release(user, 0.01)
		}()
	}
	wg.Wait()

	if got := ps.Get(user); got != 0 {
		t.Errorf("concurrent release: Get = %f, want 0", got)
	}
}
