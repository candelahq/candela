package firestoredb

// Integration tests: I1–I6 (Firestore emulator required).
// Run with: FIRESTORE_EMULATOR_HOST=localhost:8086 go test ./pkg/storage/firestoredb/...

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/candelahq/candela/pkg/storage"
)

// uniqueID generates a unique user ID for test isolation.
func uniqueID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// ====================================================================
// I1: DeductSpend auto-rollover — creates new period doc from config (#9)
// ====================================================================

// TestDeductSpend_AutoRollover_CreatesNewPeriodDoc verifies that when
// DeductSpend is called for a new billing period (no spend doc exists),
// it automatically creates a spend doc using the stable config document.
func TestDeductSpend_AutoRollover_CreatesNewPeriodDoc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("rollover-i1")
	defer cleanupUser(ctx, s, uid)

	// Create user and set budget (this writes the config doc).
	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   50.0,
		PeriodType: "daily",
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Delete the current period spend doc to simulate a new day.
	periodKey := currentPeriodKey("daily")
	_, _ = s.client.Collection(usersCol).Doc(uid).
		Collection(budgetsCol).Doc(periodKey).Delete(ctx)

	// DeductSpend must recreate the period doc from config.
	if err := s.DeductSpend(ctx, uid, 1.50, 1000); err != nil {
		t.Fatalf("DeductSpend after period rollover: %v", err)
	}

	// Verify the new spend doc was created with correct limit.
	budget, err := s.GetBudget(ctx, uid)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if budget == nil {
		t.Fatal("budget is nil after auto-rollover")
	}
	if budget.LimitUSD != 50.0 {
		t.Errorf("LimitUSD = %.2f, want 50.0 (should come from config doc)", budget.LimitUSD)
	}
	if budget.SpentUSD != 1.50 {
		t.Errorf("SpentUSD = %.2f, want 1.50", budget.SpentUSD)
	}
}

// ====================================================================
// I2: GetBudget falls back to config doc on missing period doc (#9)
// ====================================================================

// TestGetBudget_FallsBackToConfigDoc_ZeroSpend verifies that GetBudget
// returns a synthesized zero-spend record when no period doc exists.
func TestGetBudget_FallsBackToConfigDoc_ZeroSpend(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("fallback-i2")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   25.0,
		PeriodType: "daily",
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Delete the period doc to simulate a new day.
	periodKey := currentPeriodKey("daily")
	_, _ = s.client.Collection(usersCol).Doc(uid).
		Collection(budgetsCol).Doc(periodKey).Delete(ctx)

	budget, err := s.GetBudget(ctx, uid)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if budget == nil {
		t.Fatal("expected synthesized budget from config, got nil")
	}
	if budget.LimitUSD != 25.0 {
		t.Errorf("LimitUSD = %.2f, want 25.0", budget.LimitUSD)
	}
	if budget.SpentUSD != 0 {
		t.Errorf("SpentUSD = %.2f, want 0 (new period)", budget.SpentUSD)
	}
}

// ====================================================================
// I3: SetBudget writes the budgets/config document (#9)
// ====================================================================

// TestSetBudget_WritesConfigDoc verifies that SetBudget creates (or
// updates) the stable budgets/config document that the auto-rollover
// logic depends on.
func TestSetBudget_WritesConfigDoc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("config-i3")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   100.0,
		PeriodType: "daily",
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Read the config doc directly from Firestore.
	configSnap, err := s.client.Collection(usersCol).Doc(uid).
		Collection(budgetsCol).Doc(budgetConfigDocID).Get(ctx)
	if err != nil {
		t.Fatalf("getting config doc: %v", err)
	}
	data := configSnap.Data()
	if data["limit_usd"] == nil {
		t.Error("config doc missing limit_usd field")
	}
	if data["period_type"] == nil {
		t.Error("config doc missing period_type field")
	}

	// Verify the limit round-trips correctly via the helper.
	got := firestoreFloat(data["limit_usd"])
	if got != 100.0 {
		t.Errorf("config doc limit_usd = %.2f, want 100.0", got)
	}
}

// ====================================================================
// I4: GetBudget with integer limit_usd round-trips correctly (#E)
// ====================================================================

// TestGetBudget_IntegerLimitUSD_RoundTrip verifies that a budget limit
// stored as a Firestore integer (int64) is correctly read back as a
// float64. This is the fix for critical issue #E.
func TestGetBudget_IntegerLimitUSD_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("intlimit-i4")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Write the config doc directly with an integer value for limit_usd
	// (simulating what happens when Firestore stores a whole number as int64).
	configRef := s.client.Collection(usersCol).Doc(uid).
		Collection(budgetsCol).Doc(budgetConfigDocID)
	_, err := configRef.Set(ctx, map[string]interface{}{
		"limit_usd":   int64(100), // integer, not float
		"period_type": "daily",
		"user_id":     uid,
	})
	if err != nil {
		t.Fatalf("writing integer config doc: %v", err)
	}

	// GetBudget must return 100.0, not 0.0.
	budget, err := s.GetBudget(ctx, uid)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if budget == nil {
		t.Fatal("budget is nil — config doc not found")
	}
	if budget.LimitUSD != 100.0 {
		t.Errorf("LimitUSD = %.2f, want 100.0 (integer limit_usd was not read correctly)", budget.LimitUSD)
	}
}

// ====================================================================
// I5: DeductSpend tracks tokens even when CostUSD=0 (#10)
// ====================================================================

// TestDeductSpend_TokenOnlyCall_ZeroCost verifies that DeductSpend
// increments AllTokensUsed even when costUSD is 0 (e.g. local models).
func TestDeductSpend_TokenOnlyCall_ZeroCost(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("tokenonly-i5")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   50.0,
		PeriodType: "daily",
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// DeductSpend with costUSD=0 but tokens>0 (local model call).
	if err := s.DeductSpend(ctx, uid, 0.0, 500); err != nil {
		t.Fatalf("DeductSpend (zero cost): %v", err)
	}

	budget, err := s.GetBudget(ctx, uid)
	if err != nil || budget == nil {
		t.Fatalf("GetBudget: %v", err)
	}

	// SpentUSD should stay at 0, but AllTokensUsed should be 500.
	if budget.SpentUSD != 0 {
		t.Errorf("SpentUSD = %.4f, want 0.0 for a zero-cost call", budget.SpentUSD)
	}
	if budget.AllTokensUsed != 500 {
		t.Errorf("AllTokensUsed = %d, want 500", budget.AllTokensUsed)
	}
}

// ====================================================================
// I6: DeductSpend — budget exhausted does not allow overdraft
// ====================================================================

// TestDeductSpend_BudgetExhausted_DoesNotOverdraft verifies that
// DeductSpend continues to record spend even after the limit is crossed
// (the gate is at CheckBudget, not DeductSpend itself).
func TestDeductSpend_BudgetExhausted_DoesNotOverdraft(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("overdraft-i6")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   1.0,
		PeriodType: "daily",
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// First deduction: within budget.
	if err := s.DeductSpend(ctx, uid, 0.75, 100); err != nil {
		t.Fatalf("DeductSpend #1: %v", err)
	}

	// Second deduction: would push past the limit.
	if err := s.DeductSpend(ctx, uid, 0.50, 100); err != nil {
		t.Fatalf("DeductSpend #2: %v", err)
	}

	// After both calls, CheckBudget should report budget exhausted.
	check, err := s.CheckBudget(ctx, uid, 0.001)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if check.Allowed {
		t.Errorf("CheckBudget.Allowed = true after overdraft (spent=1.25 > limit=1.0), want false")
	}

	// Verify the actual spend was recorded (not capped at limit).
	budget, _ := s.GetBudget(ctx, uid)
	if budget.SpentUSD < 1.25 {
		t.Errorf("SpentUSD = %.2f, want >= 1.25 (spend is recorded even over limit)", budget.SpentUSD)
	}
}

// ====================================================================
// I7 / I8: moved to billing_test.go in pkg/proxy (proxy integration tests)
// They test the full HTTP proxy layer — living there is more appropriate.
// ====================================================================

// Ensure firestoreFloat handles all expected types (compilation guard).
func TestFirestoreFloat_TypeCoverage(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  float64
	}{
		{"float64", float64(42.5), 42.5},
		{"int64", int64(100), 100.0},
		{"int32", int32(50), 50.0},
		{"nil", nil, 0.0},
		{"string", "100", 0.0}, // invalid type → zero
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firestoreFloat(tc.input)
			if got != tc.want {
				t.Errorf("firestoreFloat(%v [%T]) = %v, want %v", tc.input, tc.input, got, tc.want)
			}
		})
	}
}

// Suppress "imported and not used" for firestore if only used in type assertions.
var _ = (*firestore.Client)(nil)
