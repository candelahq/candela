package proxy

import (
	"sync"
)

// pendingSpendTracker tracks in-flight spend reservations per user.
// This closes the TOCTOU gap between CheckBudget (pre-flight) and
// DeductSpend (post-response). Without this, concurrent requests can
// all pass CheckBudget simultaneously because none have been recorded
// in Firestore yet.
//
// Thread-safe via sync.Mutex. All amounts are in USD.
type pendingSpendTracker struct {
	mu      sync.Mutex
	pending map[string]float64 // userID → total pending USD
}

func newPendingSpendTracker() *pendingSpendTracker {
	return &pendingSpendTracker{
		pending: make(map[string]float64),
	}
}

// Reserve adds an estimated cost to the user's pending spend.
// Returns the total pending spend for this user AFTER the reservation.
func (t *pendingSpendTracker) Reserve(userID string, amountUSD float64) float64 {
	if amountUSD <= 0 || userID == "" {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[userID] += amountUSD
	return t.pending[userID]
}

// Release removes a reservation after DeductSpend completes.
// The amount is clamped so pending never goes negative.
func (t *pendingSpendTracker) Release(userID string, amountUSD float64) {
	if amountUSD <= 0 || userID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[userID] -= amountUSD
	if t.pending[userID] <= 0 {
		delete(t.pending, userID) // GC zero entries
	}
}

// Get returns the current total pending spend for a user.
func (t *pendingSpendTracker) Get(userID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending[userID]
}

// ReserveIfUnder atomically checks whether (remaining - currentPending - reserveAmount)
// stays above floor, and if so, records the reservation. Returns true if reserved.
// This prevents the TOCTOU race between separate Get and Reserve calls.
func (t *pendingSpendTracker) ReserveIfUnder(id string, remaining, reserveAmount, floor float64) bool {
	if reserveAmount <= 0 || id == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	effective := remaining - t.pending[id] - reserveAmount
	if effective < floor {
		return false
	}
	t.pending[id] += reserveAmount
	return true
}
