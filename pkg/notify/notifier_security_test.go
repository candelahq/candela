package notify

import (
	"context"
	"testing"
)

// ── Tests for audit v2 fixes ──

func TestCheckAndNotify_PeriodRolloverResetsTracking(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	// Fire 80% in period 1.
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04-01",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})
	if len(mock.alerts) != 1 {
		t.Fatalf("expected 1 alert after period 1, got %d", len(mock.alerts))
	}

	// Rollover to period 2 — 80% should fire again (map was reset).
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04-02",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})
	if len(mock.alerts) != 2 {
		t.Fatalf("expected 2 alerts after period rollover, got %d", len(mock.alerts))
	}
}

func TestCheckAndNotify_MapDoesNotGrowAcrossPeriods(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	// Simulate 100 different periods — map should never exceed a few entries.
	for i := range 100 {
		period := "2026-" + string(rune(i))
		checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", period,
			DeductResult{SpentUSD: 50.0, LimitUSD: 50.0}) // 100%
	}

	// After 100 periods × 3 thresholds = old behavior would have 300 entries.
	// With period rollover reset, should only have entries for the last period.
	checker.mu.RLock()
	mapSize := len(checker.sent)
	checker.mu.RUnlock()

	if mapSize > 10 { // generous bound: 1 user × 3 thresholds = 3, allow slack
		t.Errorf("sent map grew to %d entries across 100 periods — expected ≤10 (memory leak fix)", mapSize)
	}
}

func TestCheckAndNotify_MultipleUsersInSamePeriod(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	// Two users in same period — both should get alerts.
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04-01",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})
	checker.CheckAndNotify(context.Background(), "user-2", "bob@example.com", "2026-04-01",
		DeductResult{SpentUSD: 46.0, LimitUSD: 50.0})

	// user-1: 84% → 80%
	// user-2: 92% → 80% + 90%
	if len(mock.alerts) != 3 {
		t.Errorf("expected 3 alerts (1 for alice + 2 for bob), got %d", len(mock.alerts))
	}
}
