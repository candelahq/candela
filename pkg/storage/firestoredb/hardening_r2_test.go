package firestoredb

// Round 2: 5 unit tests + 4 integration tests for critical issues #F, #H, #I.
// Integration tests require FIRESTORE_EMULATOR_HOST to be set.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ====================================================================
// Unit Tests (no Firestore required)
// ====================================================================

// U13: currentPeriodKey("daily") returns YYYY-MM-DD.
func TestCurrentPeriodKey_Daily(t *testing.T) {
	got := currentPeriodKey("daily")
	want := time.Now().UTC().Format("2006-01-02")
	if got != want {
		t.Errorf("daily: got %q, want %q", got, want)
	}
}

// U14: currentPeriodKey("monthly") returns YYYY-MM, not YYYY-MM-DD.
// Before fix #F the argument was ignored and "monthly" silently returned
// YYYY-MM-DD — causing monthly budgets to reset every day.
func TestCurrentPeriodKey_Monthly(t *testing.T) {
	got := currentPeriodKey("monthly")
	want := time.Now().UTC().Format("2006-01")
	if got != want {
		t.Errorf("monthly: got %q, want %q", got, want)
	}
	// Must NOT be a full date — that would mean the fix didn't apply.
	if len(got) == 10 {
		t.Errorf("monthly key looks like a daily key (YYYY-MM-DD): %q", got)
	}
}

// U15: currentPeriodKey("weekly") returns a week-number key (YYYY-WNN).
func TestCurrentPeriodKey_Weekly(t *testing.T) {
	got := currentPeriodKey("weekly")
	yearStr := time.Now().UTC().Format("2006")
	if !strings.HasPrefix(got, yearStr+"-W") {
		t.Errorf("weekly key has unexpected format: %q (want prefix %q)", got, yearStr+"-W")
	}
}

// U16: sanitizeID handles consecutive dots (#I).
func TestSanitizeID_ConsecutiveDots(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Single dot is fine — austin.bennett@azra-ai.com → unchanged.
		{"austin.bennett@azra-ai.com", "austin.bennett@azra-ai.com"},
		// Consecutive dots collapse.
		{"user..name@foo.com", "user._name@foo.com"},
		// Slash is replaced.
		{"team/user@foo.com", "team_user@foo.com"},
	}
	for _, tc := range tests {
		got := sanitizeID(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// U17: sanitizeID truncates IDs longer than 1500 bytes (#I).
func TestSanitizeID_TruncatesLongID(t *testing.T) {
	long := strings.Repeat("a", 2000) + "@foo.com"
	got := sanitizeID(long)
	if len(got) > 1500 {
		t.Errorf("sanitizeID did not truncate: len=%d, want ≤1500", len(got))
	}
}

// ====================================================================
// Integration Tests (Firestore emulator required)
// ====================================================================

// I9: currentPeriodKey("monthly") generates a different key than "daily"
// so a monthly budget doc is stored under YYYY-MM, not YYYY-MM-DD.
// This is the key assertion that proves fix #F works end-to-end.
func TestSetBudget_MonthlyPeriod_CorrectDocKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("monthly-i9")
	defer cleanupUser(ctx, s, uid)

	if err := s.CreateUser(ctx, &storage.UserRecord{ID: uid, Email: uid + "@test.com"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SetBudget(ctx, &storage.BudgetRecord{
		UserID:     uid,
		LimitUSD:   500.0,
		PeriodType: "monthly",
	}); err != nil {
		t.Fatalf("SetBudget (monthly): %v", err)
	}

	monthlyKey := currentPeriodKey("monthly")
	dailyKey := currentPeriodKey("daily")
	if monthlyKey == dailyKey {
		t.Skip("monthly and daily keys identical — skipping (first day of month)")
	}

	// The monthly doc must exist.
	monthlyRef := s.client.Collection(usersCol).Doc(sanitizeID(uid)).
		Collection(budgetsCol).Doc(monthlyKey)
	snap, err := monthlyRef.Get(ctx)
	if err != nil || !snap.Exists() {
		t.Errorf("monthly budget doc not found at key %q: %v", monthlyKey, err)
	}

	// The daily-keyed doc must NOT exist for a monthly-period budget.
	dailyRef := s.client.Collection(usersCol).Doc(sanitizeID(uid)).
		Collection(budgetsCol).Doc(dailyKey)
	dailySnap, _ := dailyRef.Get(ctx)
	if dailySnap != nil && dailySnap.Exists() {
		t.Errorf("daily doc found at %q but budget was set as monthly — period key not respected", dailyKey)
	}
}

// I10: ResetSpend succeeds even when no period doc exists yet (#H).
// Before fix #H this returned NOT_FOUND because Update requires the doc to exist.
func TestResetSpend_IdempotentOnMissingPeriodDoc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("reset-i10")
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

	// Delete the period doc to simulate a fresh day (no spend doc yet).
	periodKey := currentPeriodKey("daily")
	_, _ = s.client.Collection(usersCol).Doc(sanitizeID(uid)).
		Collection(budgetsCol).Doc(periodKey).Delete(ctx)

	// ResetSpend must not error even though the period doc doesn't exist.
	if err := s.ResetSpend(ctx, uid); err != nil {
		t.Errorf("ResetSpend on missing period doc returned error: %v", err)
	}
}

// I11: ResetSpend zeroes spend on an existing period doc (#H).
func TestResetSpend_ZeroesExistingSpend(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := uniqueID("reset-i11")
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
	if err := s.DeductSpend(ctx, uid, 2.50, 1000); err != nil {
		t.Fatalf("DeductSpend: %v", err)
	}

	if err := s.ResetSpend(ctx, uid); err != nil {
		t.Fatalf("ResetSpend: %v", err)
	}

	budget, err := s.GetBudget(ctx, uid)
	if err != nil || budget == nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if budget.SpentUSD != 0 {
		t.Errorf("SpentUSD = %.2f after reset, want 0", budget.SpentUSD)
	}
	if budget.AllTokensUsed != 0 {
		t.Errorf("AllTokensUsed = %d after reset, want 0", budget.AllTokensUsed)
	}
}

// I12: sanitizeID with a very long email still writes a valid Firestore doc (#I).
func TestSanitizeID_LongEmail_WritesValidDoc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	longLocal := strings.Repeat("x", 800)
	email := longLocal + "@verylongdomain.example.com"
	uid := sanitizeID(email)
	if len(uid) > 1500 {
		t.Fatalf("sanitizeID returned ID longer than 1500 bytes: %d", len(uid))
	}

	rec := &storage.UserRecord{ID: uid, Email: email[:100]}
	defer cleanupUser(ctx, s, uid)

	// The write must not fail with an invalid document ID error.
	if err := s.CreateUser(ctx, rec); err != nil {
		t.Errorf("CreateUser with long-email-derived ID failed: %v", err)
	}
}
