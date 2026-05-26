package firestoredb

// Unit tests for billing/counting fixes: CRIT-11, CRIT-12, CRIT-14, CRIT-15.
// Note: TestCurrentPeriodKey_Daily/Monthly/Weekly already exist in hardening_r2_test.go.
// This file adds tests for the new behaviour introduced by these fixes.

import (
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── CRIT-14: period type propagates through the full converter chain ─────────

func TestCurrentPeriodKey_UnknownFallsBackToDaily(t *testing.T) {
	key := currentPeriodKey("quarterly") // unknown → daily fallback
	if len(key) != 10 {
		t.Errorf("unknown type key = %q, want daily fallback YYYY-MM-DD (10 chars)", key)
	}
}

func TestCurrentPeriodKey_DailyAndMonthlyAreDifferent(t *testing.T) {
	// Regression for CRIT-14: if period_type is ignored, both return the same
	// key and monthly budgets behave identically to daily budgets.
	if currentPeriodKey("daily") == currentPeriodKey("monthly") {
		t.Error("daily key == monthly key — period_type is being ignored")
	}
}

func TestCurrentPeriodKey_DailyAndWeeklyAreDifferent(t *testing.T) {
	if currentPeriodKey("daily") == currentPeriodKey("weekly") {
		t.Error("daily key == weekly key — period_type is being ignored")
	}
}

// ─── CRIT-11: budgetToFirestore preserves non-daily period type ───────────────
// The auto-rollover path (isNewPeriod) was the missed call site — it used to
// call tx.Set(budgetRef, rawBudgetRecord) before CRIT-11 was fixed.

func TestBudgetToFirestore_MonthlyPeriod(t *testing.T) {
	b := &storage.BudgetRecord{
		UserID:        "alice@example.com",
		LimitUSD:      100.0,
		SpentUSD:      42.5,
		TokensUsed:    1000,
		AllTokensUsed: 1500,
		PeriodType:    "monthly",
		PeriodKey:     "2026-05",
	}
	fb := budgetToFirestore(b)
	if fb.PeriodType != "monthly" {
		t.Errorf("PeriodType = %q, want monthly", fb.PeriodType)
	}
	if fb.PeriodKey != "2026-05" {
		t.Errorf("PeriodKey = %q, want 2026-05", fb.PeriodKey)
	}
	if fb.LimitUSD != 100.0 {
		t.Errorf("LimitUSD = %f, want 100.0", fb.LimitUSD)
	}
	if fb.SpentUSD != 42.5 {
		t.Errorf("SpentUSD = %f, want 42.5", fb.SpentUSD)
	}
}

func TestBudgetToFirestore_WeeklyPeriod(t *testing.T) {
	b := &storage.BudgetRecord{
		UserID:     "bob@example.com",
		LimitUSD:   50.0,
		PeriodType: "weekly",
		PeriodKey:  "2026-W19",
	}
	fb := budgetToFirestore(b)
	if fb.PeriodType != "weekly" {
		t.Errorf("PeriodType = %q, want weekly", fb.PeriodType)
	}
	if fb.PeriodKey != "2026-W19" {
		t.Errorf("PeriodKey = %q, want 2026-W19", fb.PeriodKey)
	}
}

func TestFirestoreToBudget_MonthlyRoundTrip(t *testing.T) {
	// Regression for CRIT-14: monthly period_type must survive the
	// budgetToFirestore → firestoreToBudget round-trip.
	original := &storage.BudgetRecord{
		UserID:     "carol@example.com",
		LimitUSD:   200.0,
		PeriodType: "monthly",
		PeriodKey:  "2026-05",
	}
	got := firestoreToBudget(budgetToFirestore(original))
	if got.PeriodType != "monthly" {
		t.Errorf("PeriodType after round-trip = %q, want monthly", got.PeriodType)
	}
	if got.PeriodKey != "2026-05" {
		t.Errorf("PeriodKey after round-trip = %q, want 2026-05", got.PeriodKey)
	}
}

// ─── CRIT-15: StatusInactive constant ────────────────────────────────────────

func TestStatusInactiveMatchesGuard(t *testing.T) {
	// CheckBudget compares user.Status == storage.StatusInactive.
	// Verify the constant is exactly "inactive" — the value stored in Firestore.
	if storage.StatusInactive != "inactive" {
		t.Errorf("storage.StatusInactive = %q, want \"inactive\"", storage.StatusInactive)
	}
}

// ─── CRIT-12: grant overdraft behaviour documentation ────────────────────────

func TestGrantRecord_Remaining_Overdraft(t *testing.T) {
	// BILL-3: Remaining() is now clamped to 0 when SpentUSD > AmountUSD.
	// Previously negative values could reduce the apparent total budget in
	// CheckBudget (grantsRemaining += g.Remaining()), causing premature blocking.
	g := &storage.GrantRecord{AmountUSD: 10.0, SpentUSD: 15.0}
	if got := g.Remaining(); got != 0.0 {
		t.Errorf("Remaining() on overdraft = %f, want 0.0 (clamped)", got)
	}
}

func TestGrantRecord_Remaining_ExactlyZero(t *testing.T) {
	g := &storage.GrantRecord{AmountUSD: 25.0, SpentUSD: 25.0}
	if got := g.Remaining(); got != 0.0 {
		t.Errorf("Remaining() on fully-spent grant = %f, want 0.0", got)
	}
}
