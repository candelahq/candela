package firestoredb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/candelahq/candela/pkg/storage"
)

// requireEmulator skips the test if the Firestore emulator is not running.
// Start it with: gcloud emulators firestore start --host-port=localhost:8086
func requireEmulator(t *testing.T) {
	t.Helper()
	host := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if host == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set — skipping Firestore tests (start emulator with: gcloud emulators firestore start)")
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	requireEmulator(t)

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "test-project")
	if err != nil {
		t.Fatalf("failed to create Firestore client: %v", err)
	}

	store := NewWithClient(client)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// cleanupUser deletes a user and all subcollections for test isolation.
func cleanupUser(ctx context.Context, s *Store, userID string) {
	ref := s.client.Collection(usersCol).Doc(userID)
	for _, sub := range []string{budgetsCol, grantsCol, auditCol} {
		iter := ref.Collection(sub).Documents(ctx)
		snaps, _ := iter.GetAll()
		for _, snap := range snaps {
			_, _ = snap.Ref.Delete(ctx)
		}
	}
	_, _ = ref.Delete(ctx)
}

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &storage.UserRecord{
		ID:          fmt.Sprintf("test-user-%d", time.Now().UnixNano()),
		Email:       "alice@example.com",
		DisplayName: "Alice",
		Role:        "developer",
	}
	t.Cleanup(func() { cleanupUser(ctx, s, user.ID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "alice@example.com")
	}
	if got.Status != "provisioned" {
		t.Errorf("Status = %q, want %q", got.Status, "provisioned")
	}
}

func TestGetUserByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &storage.UserRecord{
		ID:    fmt.Sprintf("test-email-%d", time.Now().UnixNano()),
		Email: fmt.Sprintf("bob-%d@example.com", time.Now().UnixNano()),
		Role:  "developer",
	}
	t.Cleanup(func() { cleanupUser(ctx, s, user.ID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID = %q, want %q", got.ID, user.ID)
	}
}

func TestTouchLastSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &storage.UserRecord{
		ID:    fmt.Sprintf("test-touch-%d", time.Now().UnixNano()),
		Email: "touch@example.com",
		Role:  "developer",
	}
	t.Cleanup(func() { cleanupUser(ctx, s, user.ID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.TouchLastSeen(ctx, user.ID); err != nil {
		t.Fatalf("TouchLastSeen: %v", err)
	}

	got, _ := s.GetUser(ctx, user.ID)
	if got.Status != "active" {
		t.Errorf("Status after touch = %q, want %q", got.Status, "active")
	}
	if got.LastSeenAt.IsZero() {
		t.Error("LastSeenAt should not be zero after touch")
	}
}

func TestSetAndGetBudget(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-budget-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "budget@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	budget := &storage.BudgetRecord{
		UserID:     userID,
		LimitUSD:   100.0,
		PeriodType: "daily",
	}
	if err := s.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	got, err := s.GetBudget(ctx, userID)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if got == nil {
		t.Fatal("GetBudget returned nil")
	}
	if got.LimitUSD != 100.0 {
		t.Errorf("LimitUSD = %f, want 100.0", got.LimitUSD)
	}
}

func TestGrantLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-grant-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "grant@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Create a grant.
	grant := &storage.GrantRecord{
		UserID:    userID,
		AmountUSD: 50.0,
		Reason:    "test grant",
		GrantedBy: "admin@example.com",
		StartsAt:  time.Now().UTC().Add(-1 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := s.CreateGrant(ctx, grant); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	// List active grants.
	grants, err := s.ListGrants(ctx, userID, true)
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 active grant, got %d", len(grants))
	}
	if grants[0].AmountUSD != 50.0 {
		t.Errorf("grant amount = %f, want 50.0", grants[0].AmountUSD)
	}

	// Revoke the grant.
	if err := s.RevokeGrant(ctx, userID, grants[0].ID); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	// Should have no active grants now.
	grants, err = s.ListGrants(ctx, userID, true)
	if err != nil {
		t.Fatalf("ListGrants after revoke: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected 0 active grants after revoke, got %d", len(grants))
	}
}

func TestCheckBudget(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-check-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "check@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Set $100 daily budget.
	budget := &storage.BudgetRecord{
		UserID: userID, LimitUSD: 100.0, PeriodType: "daily",
	}
	if err := s.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Add $20 grant.
	grant := &storage.GrantRecord{
		UserID: userID, AmountUSD: 20.0, Reason: "test",
		GrantedBy: "admin@example.com",
		StartsAt:  time.Now().UTC().Add(-time.Hour),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := s.CreateGrant(ctx, grant); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	// Check: $50 should be allowed (20 grant + 100 budget = 120 remaining).
	result, err := s.CheckBudget(ctx, userID, 50.0)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if !result.Allowed {
		t.Error("expected $50 check to be allowed")
	}
	if result.RemainingUSD != 120.0 {
		t.Errorf("RemainingUSD = %f, want 120.0", result.RemainingUSD)
	}

	// Check: $200 should be denied.
	result, err = s.CheckBudget(ctx, userID, 200.0)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if result.Allowed {
		t.Error("expected $200 check to be denied")
	}
}

func TestDeductSpend_GrantFirstWaterfall(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-deduct-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "deduct@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Set $100 daily budget.
	budget := &storage.BudgetRecord{
		UserID: userID, LimitUSD: 100.0, PeriodType: "daily",
	}
	if err := s.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Create a $10 grant (expires sooner) and a $20 grant (expires later).
	grant1 := &storage.GrantRecord{
		UserID: userID, AmountUSD: 10.0, Reason: "small grant",
		GrantedBy: "admin@example.com",
		StartsAt:  time.Now().UTC().Add(-time.Hour),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour), // expires first
	}
	grant2 := &storage.GrantRecord{
		UserID: userID, AmountUSD: 20.0, Reason: "big grant",
		GrantedBy: "admin@example.com",
		StartsAt:  time.Now().UTC().Add(-time.Hour),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour), // expires later
	}
	if err := s.CreateGrant(ctx, grant1); err != nil {
		t.Fatalf("CreateGrant 1: %v", err)
	}
	if err := s.CreateGrant(ctx, grant2); err != nil {
		t.Fatalf("CreateGrant 2: %v", err)
	}

	// Deduct $25 — should consume grant1 ($10) + grant2 ($15), monthly budget untouched.
	if err := s.DeductSpend(ctx, userID, 25.0, 1000); err != nil {
		t.Fatalf("DeductSpend: %v", err)
	}

	// Check grant1: should be fully spent.
	grants, _ := s.ListGrants(ctx, userID, false)
	for _, g := range grants {
		if g.Reason == "small grant" && g.SpentUSD != 10.0 {
			t.Errorf("small grant spent = %f, want 10.0", g.SpentUSD)
		}
		if g.Reason == "big grant" && g.SpentUSD != 15.0 {
			t.Errorf("big grant spent = %f, want 15.0", g.SpentUSD)
		}
	}

	// Check budget: should be untouched.
	b, _ := s.GetBudget(ctx, userID)
	if b.SpentUSD != 0 {
		t.Errorf("budget spent = %f, want 0 (all deducted from grants)", b.SpentUSD)
	}

	// Deduct another $10 — should consume remaining grant2 ($5) + budget ($5).
	if err := s.DeductSpend(ctx, userID, 10.0, 500); err != nil {
		t.Fatalf("DeductSpend 2: %v", err)
	}

	b, _ = s.GetBudget(ctx, userID)
	if b.SpentUSD != 5.0 {
		t.Errorf("budget spent after second deduct = %f, want 5.0", b.SpentUSD)
	}
	// Token attribution: $10 cost, $5 from grant2, $5 from budget.
	// Budget absorbed 50% of the cost, so should get 50% of 500 tokens = 250.
	if b.TokensUsed != 250 {
		t.Errorf("budget tokens after proportional split = %d, want 250 (50%% of 500)", b.TokensUsed)
	}
}

func TestAuditLog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-audit-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "audit@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Log two actions.
	for i, action := range []string{"set_budget", "create_grant"} {
		entry := &storage.AuditRecord{
			UserID:     userID,
			ActorEmail: "admin@example.com",
			Action:     action,
			Details:    fmt.Sprintf(`{"test": %d}`, i),
		}
		if err := s.LogAction(ctx, entry); err != nil {
			t.Fatalf("LogAction %d: %v", i, err)
		}
	}

	entries, err := s.ListAuditLog(ctx, userID, 10)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}
	// Should be ordered newest-first.
	if entries[0].Action != "create_grant" {
		t.Errorf("first entry action = %q, want %q (newest first)", entries[0].Action, "create_grant")
	}
}

func TestCurrentPeriodKey(t *testing.T) {
	// All budgets are daily — verify the format is YYYY-MM-DD.
	daily := currentPeriodKey("daily")
	if len(daily) != 10 { // "2026-04-21"
		t.Errorf("daily key = %q, expected format like 2026-04-21", daily)
	}

	// Any input should return daily format.
	also := currentPeriodKey("anything")
	if also != daily {
		t.Errorf("expected same daily key for any input, got %q vs %q", also, daily)
	}
}

func TestResetSpend(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-reset-%d", time.Now().UnixNano())
	user := &storage.UserRecord{ID: userID, Email: "reset@example.com", Role: "developer"}
	t.Cleanup(func() { cleanupUser(ctx, s, userID) })
	_ = s.CreateUser(ctx, user)

	// Set budget with some spend.
	budget := &storage.BudgetRecord{
		UserID: userID, LimitUSD: 100.0, SpentUSD: 42.0,
		TokensUsed: 5000, PeriodType: "daily",
	}
	_ = s.SetBudget(ctx, budget)

	// Reset.
	if err := s.ResetSpend(ctx, userID); err != nil {
		t.Fatalf("ResetSpend: %v", err)
	}

	got, _ := s.GetBudget(ctx, userID)
	if got.SpentUSD != 0 {
		t.Errorf("spent_usd after reset = %f, want 0", got.SpentUSD)
	}
	if got.TokensUsed != 0 {
		t.Errorf("tokens_used after reset = %d, want 0", got.TokensUsed)
	}
}
