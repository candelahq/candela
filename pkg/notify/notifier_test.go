package notify

import (
	"context"
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

// mockNotifier captures notifications for assertions.
type mockNotifier struct {
	alerts []storage.BudgetAlert
}

func (m *mockNotifier) NotifyBudgetThreshold(_ context.Context, alert storage.BudgetAlert) error {
	m.alerts = append(m.alerts, alert)
	return nil
}

func TestCheckAndNotify_FiresAtThreshold(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0}) // 84% → should fire 80%

	if len(mock.alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(mock.alerts))
	}
	if mock.alerts[0].Threshold != 0.80 {
		t.Errorf("expected 80%% threshold, got %.2f", mock.alerts[0].Threshold)
	}
	if mock.alerts[0].Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", mock.alerts[0].Email)
	}
}

func TestCheckAndNotify_MultpleThresholds(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	// Jump from 0 to 95% — should fire both 80% and 90%
	checker.CheckAndNotify(context.Background(), "user-1", "bob@example.com", "2026-04",
		DeductResult{SpentUSD: 47.5, LimitUSD: 50.0})

	if len(mock.alerts) != 2 {
		t.Fatalf("expected 2 alerts (80%% + 90%%), got %d", len(mock.alerts))
	}
	if mock.alerts[0].Threshold != 0.80 {
		t.Errorf("first alert: expected 80%%, got %.2f", mock.alerts[0].Threshold)
	}
	if mock.alerts[1].Threshold != 0.90 {
		t.Errorf("second alert: expected 90%%, got %.2f", mock.alerts[1].Threshold)
	}
}

func TestCheckAndNotify_NoDuplicates(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	// Fire 80% threshold
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})

	// Same threshold again — should not fire
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 43.0, LimitUSD: 50.0})

	if len(mock.alerts) != 1 {
		t.Errorf("expected 1 alert (no duplicate), got %d", len(mock.alerts))
	}
}

func TestCheckAndNotify_DifferentPeriod(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})

	// New period — 80% should fire again
	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-05",
		DeductResult{SpentUSD: 42.0, LimitUSD: 50.0})

	if len(mock.alerts) != 2 {
		t.Errorf("expected 2 alerts (different periods), got %d", len(mock.alerts))
	}
}

func TestCheckAndNotify_BelowThreshold(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 30.0, LimitUSD: 50.0}) // 60% — below 80%

	if len(mock.alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(mock.alerts))
	}
}

func TestCheckAndNotify_ZeroLimit(t *testing.T) {
	mock := &mockNotifier{}
	checker := NewBudgetChecker(mock)

	checker.CheckAndNotify(context.Background(), "user-1", "alice@example.com", "2026-04",
		DeductResult{SpentUSD: 10.0, LimitUSD: 0}) // no budget — no alerts

	if len(mock.alerts) != 0 {
		t.Errorf("expected 0 alerts for zero limit, got %d", len(mock.alerts))
	}
}

func TestDeductResult_Ratio(t *testing.T) {
	tests := []struct {
		name     string
		result   DeductResult
		expected float64
	}{
		{"normal", DeductResult{SpentUSD: 40, LimitUSD: 50}, 0.80},
		{"zero limit", DeductResult{SpentUSD: 10, LimitUSD: 0}, 0},
		{"overspent", DeductResult{SpentUSD: 60, LimitUSD: 50}, 1.20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Ratio()
			if got != tt.expected {
				t.Errorf("Ratio() = %v, want %v", got, tt.expected)
			}
		})
	}
}
