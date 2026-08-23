package proxy

import (
	"sync"
	"sync/atomic"
	"testing"

	"pgregory.net/rapid"
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

// ── ReserveIfUnder ──

func TestReserveIfUnder_AcceptsWithinBudget(t *testing.T) {
	ps := newPendingSpendTracker()
	ok := ps.ReserveIfUnder("job-1", 10.0, 0.05, 0.001)
	if !ok {
		t.Fatal("ReserveIfUnder should accept: remaining=$10, reserve=$0.05")
	}
	if got := ps.Get("job-1"); got != 0.05 {
		t.Errorf("pending = %f, want 0.05", got)
	}
}

func TestReserveIfUnder_RejectsOverBudget(t *testing.T) {
	ps := newPendingSpendTracker()
	// remaining=$0.04, reserve=$0.05, floor=$0.001 → effective = -0.01 < floor
	ok := ps.ReserveIfUnder("job-2", 0.04, 0.05, 0.001)
	if ok {
		t.Fatal("ReserveIfUnder should reject: remaining=$0.04 < reserve=$0.05")
	}
	if got := ps.Get("job-2"); got != 0 {
		t.Errorf("pending = %f, want 0 (rejected)", got)
	}
}

func TestReserveIfUnder_AccountsForExistingPending(t *testing.T) {
	ps := newPendingSpendTracker()
	// First reservation eats into remaining
	ps.Reserve("job-3", 9.95)
	// remaining=$10.0, pending=$9.95, reserve=$0.05 → effective = 0.0 < floor
	ok := ps.ReserveIfUnder("job-3", 10.0, 0.05, 0.001)
	if ok {
		t.Fatal("ReserveIfUnder should reject: effective=$0.0 < floor=$0.001")
	}
}

func TestReserveIfUnder_ExactFloorAccepted(t *testing.T) {
	ps := newPendingSpendTracker()
	// remaining=$1.0, reserve=$0.999 → effective = 0.001 == floor → accepted
	ok := ps.ReserveIfUnder("job-4", 1.0, 0.999, 0.001)
	if !ok {
		t.Fatal("ReserveIfUnder should accept: effective=$0.001 == floor")
	}
}

func TestReserveIfUnder_ZeroInputs(t *testing.T) {
	ps := newPendingSpendTracker()
	if ps.ReserveIfUnder("", 10.0, 0.05, 0.001) {
		t.Error("empty ID should return false")
	}
	if ps.ReserveIfUnder("job", 10.0, 0, 0.001) {
		t.Error("zero reserve should return false")
	}
	if ps.ReserveIfUnder("job", 10.0, -1, 0.001) {
		t.Error("negative reserve should return false")
	}
}

func TestReserveIfUnder_ConcurrentAdmission(t *testing.T) {
	// Simulate 20 concurrent subagents each trying to reserve $0.05
	// against a $0.50 budget. At most 10 should be admitted
	// (10 × $0.05 = $0.50, effective = $0.50 - $0.50 = $0.00 < floor).
	ps := newPendingSpendTracker()
	const (
		remaining   = 0.50
		reserveEach = 0.05
		floor       = 0.001
		goroutines  = 20
		maxAdmitted = 9 // (0.50 - 9*0.05) = 0.05 ≥ 0.001; 10th: (0.50-0.50)=0.00 < 0.001
	)

	var wg sync.WaitGroup
	var admitted atomic.Int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ps.ReserveIfUnder("job-race", remaining, reserveEach, floor) {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()

	got := admitted.Load()
	if got > maxAdmitted+1 { // +1 for float64 edge case
		t.Errorf("admitted %d goroutines, want ≤ %d (budget=$%.2f, each=$%.2f)",
			got, maxAdmitted+1, remaining, reserveEach)
	}
	if got == 0 {
		t.Error("no goroutines admitted — at least 1 should pass")
	}
	t.Logf("admitted %d/%d goroutines (budget=$%.2f, each=$%.2f)", got, goroutines, remaining, reserveEach)
}

// ── Property-based tests (rapid) ──

func TestProperty_ReserveRelease_NetZero(t *testing.T) {
	// Property: for any sequence of N reserves of amount A, followed by
	// N releases of amount A, the balance returns to zero (within float64 tolerance).
	rapid.Check(t, func(t *rapid.T) {
		ps := newPendingSpendTracker()
		id := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "id")
		n := rapid.IntRange(1, 50).Draw(t, "n")
		amount := rapid.Float64Range(0.001, 100.0).Draw(t, "amount")

		for i := 0; i < n; i++ {
			ps.Reserve(id, amount)
		}
		for i := 0; i < n; i++ {
			ps.Release(id, amount)
		}
		if got := ps.Get(id); got > 1e-9 {
			t.Fatalf("net-zero violated: after %d reserve+release of $%.4f, got $%.12f", n, amount, got)
		}
	})
}

func TestProperty_ReserveIfUnder_NeverExceedsBudget(t *testing.T) {
	// Property: for any remaining balance and floor, the total admitted
	// reservations never exceed (remaining - floor).
	rapid.Check(t, func(t *rapid.T) {
		ps := newPendingSpendTracker()
		remaining := rapid.Float64Range(0.01, 100.0).Draw(t, "remaining")
		floor := rapid.Float64Range(0.001, remaining/2).Draw(t, "floor")
		reserve := rapid.Float64Range(floor, remaining).Draw(t, "reserve")
		n := rapid.IntRange(1, 100).Draw(t, "attempts")
		id := "prop-test"

		var totalReserved float64
		for i := 0; i < n; i++ {
			if ps.ReserveIfUnder(id, remaining, reserve, floor) {
				totalReserved += reserve
			}
		}
		maxAllowed := remaining - floor
		if totalReserved > maxAllowed+0.0001 { // float64 tolerance
			t.Fatalf("overcommitted: reserved $%.4f > max $%.4f (remaining=$%.4f, floor=$%.4f, each=$%.4f)",
				totalReserved, maxAllowed, remaining, floor, reserve)
		}
	})
}

func TestProperty_PendingNeverNegative(t *testing.T) {
	// Property: Get() never returns a negative value, even after
	// releasing more than was reserved.
	rapid.Check(t, func(t *rapid.T) {
		ps := newPendingSpendTracker()
		id := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "id")
		reserves := rapid.IntRange(0, 20).Draw(t, "reserves")
		releases := rapid.IntRange(0, 30).Draw(t, "releases")
		amount := rapid.Float64Range(0.001, 10.0).Draw(t, "amount")

		for i := 0; i < reserves; i++ {
			ps.Reserve(id, amount)
		}
		for i := 0; i < releases; i++ {
			ps.Release(id, amount)
		}
		if got := ps.Get(id); got < 0 {
			t.Fatalf("negative pending: $%.4f after %d reserves, %d releases of $%.4f",
				got, reserves, releases, amount)
		}
	})
}
