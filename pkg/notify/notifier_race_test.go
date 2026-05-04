package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

// countingNotifier tracks how many times each threshold was notified.
type countingNotifier struct {
	mu    sync.Mutex
	count map[float64]int
}

func newCountingNotifier() *countingNotifier {
	return &countingNotifier{count: make(map[float64]int)}
}

func (n *countingNotifier) NotifyBudgetThreshold(_ context.Context, alert storage.BudgetAlert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.count[alert.Threshold]++
	return nil
}

func (n *countingNotifier) getCount(threshold float64) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.count[threshold]
}

func (n *countingNotifier) totalCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	total := 0
	for _, c := range n.count {
		total += c
	}
	return total
}

// TestCheckAndNotify_ConcurrentPeriodRollover runs many goroutines calling
// CheckAndNotify with alternating period keys to detect race conditions.
// Run with: go test -race ./pkg/notify/
func TestCheckAndNotify_ConcurrentPeriodRollover(t *testing.T) {
	notifier := newCountingNotifier()
	checker := NewBudgetChecker(notifier)

	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Alternate between periods to trigger rollovers.
				period := "2026-01"
				if i%3 == 0 {
					period = "2026-02"
				}
				checker.CheckAndNotify(
					context.Background(),
					"user-race",
					"race@test.com",
					period,
					DeductResult{SpentUSD: 95, LimitUSD: 100},
				)
			}
		}(g)
	}

	wg.Wait()

	// If there's a race, the race detector will catch it.
	// We just verify that notifications were sent.
	total := notifier.totalCount()
	if total == 0 {
		t.Error("expected at least some notifications to fire")
	}
	t.Logf("total notifications fired: %d (across %d goroutines × %d iterations)",
		total, goroutines, iterations)
}

// TestCheckAndNotify_NoDuplicatesAcrossRollover verifies that the period
// rollover reset doesn't cause duplicate notifications for the same threshold
// in the same period.
func TestCheckAndNotify_NoDuplicatesAcrossRollover(t *testing.T) {
	notifier := newCountingNotifier()
	checker := NewBudgetChecker(notifier)

	result := DeductResult{SpentUSD: 95, LimitUSD: 100}

	// First call: should fire 80% and 90% thresholds.
	checker.CheckAndNotify(context.Background(), "user-dup", "dup@test.com", "2026-01", result)

	count80 := notifier.getCount(0.80)
	count90 := notifier.getCount(0.90)
	if count80 != 1 {
		t.Errorf("80%% threshold fired %d times, want 1", count80)
	}
	if count90 != 1 {
		t.Errorf("90%% threshold fired %d times, want 1", count90)
	}

	// Second call: same period, same user → no duplicates.
	checker.CheckAndNotify(context.Background(), "user-dup", "dup@test.com", "2026-01", result)

	count80After := notifier.getCount(0.80)
	count90After := notifier.getCount(0.90)
	if count80After != 1 {
		t.Errorf("80%% threshold fired %d times after repeat, want 1", count80After)
	}
	if count90After != 1 {
		t.Errorf("90%% threshold fired %d times after repeat, want 1", count90After)
	}

	// Concurrent hammering with the same period should not produce duplicates.
	var duplicates atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			checker.CheckAndNotify(context.Background(), "user-dup", "dup@test.com", "2026-01", result)
		}()
	}
	wg.Wait()

	finalCount80 := notifier.getCount(0.80)
	if finalCount80 > 1 {
		duplicates.Add(int32(finalCount80 - 1))
	}

	if d := duplicates.Load(); d > 0 {
		t.Errorf("got %d duplicate 80%% notifications", d)
	}
}
