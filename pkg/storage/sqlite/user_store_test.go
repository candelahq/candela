package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
	"github.com/candelahq/candela/pkg/storage/sqlite"
)

func setupTestUserStore(t *testing.T) *sqlite.UserStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-users.db")
	store, err := sqlite.NewUserStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test UserStore: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestUserStore_UserCRUD(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	// 1. CreateUser
	displayName := "Alice Doe"
	rateLimit := 120
	user := &storage.UserRecord{
		ID:          "alice@example.com",
		Email:       "alice@example.com",
		DisplayName: &displayName,
		Role:        storage.RoleDeveloper,
		Status:      storage.StatusProvisioned,
		AccessTags:  []string{"pro", "preview"},
		RateLimit:   &rateLimit,
	}

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Duplicate create should fail
	if err := s.CreateUser(ctx, user); err == nil {
		t.Fatal("expected error creating duplicate user, got nil")
	}

	// 2. GetUser & GetUserByEmail
	got, err := s.GetUser(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got.ID != "alice@example.com" || got.Email != "alice@example.com" {
		t.Errorf("unexpected user: %+v", got)
	}
	if got.DisplayName == nil || *got.DisplayName != "Alice Doe" {
		t.Errorf("unexpected display name: %v", got.DisplayName)
	}
	if len(got.AccessTags) != 2 || got.AccessTags[0] != "pro" {
		t.Errorf("unexpected access tags: %v", got.AccessTags)
	}

	gotByEmail, err := s.GetUserByEmail(ctx, "ALICE@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("GetUserByEmail case-insensitive failed: %v", err)
	}
	if gotByEmail.ID != "alice@example.com" {
		t.Errorf("unexpected user from email: %+v", gotByEmail)
	}

	// 3. UpdateUser
	newDisplay := "Alice Smith"
	update := &storage.UserRecord{
		ID:          "alice@example.com",
		DisplayName: &newDisplay,
		Status:      storage.StatusActive,
		AccessTags:  []string{"enterprise"},
	}
	if err := s.UpdateUser(ctx, update); err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	updated, err := s.GetUser(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUser after update failed: %v", err)
	}
	if *updated.DisplayName != "Alice Smith" || updated.Status != storage.StatusActive {
		t.Errorf("updated fields mismatch: %+v", updated)
	}
	if len(updated.AccessTags) != 1 || updated.AccessTags[0] != "enterprise" {
		t.Errorf("updated access tags mismatch: %v", updated.AccessTags)
	}

	// 4. TouchLastSeen & TouchLastActive
	if err := s.TouchLastSeen(ctx, "alice@example.com"); err != nil {
		t.Fatalf("TouchLastSeen failed: %v", err)
	}
	if err := s.TouchLastActive(ctx, "alice@example.com"); err != nil {
		t.Fatalf("TouchLastActive failed: %v", err)
	}

	touched, _ := s.GetUser(ctx, "alice@example.com")
	if touched.LastSeenAt.IsZero() || touched.LastActiveAt.IsZero() {
		t.Errorf("timestamps not touched: seen=%v, active=%v", touched.LastSeenAt, touched.LastActiveAt)
	}

	// 5. ListUsers & GetUsers
	users, total, err := s.ListUsers(ctx, "", 10, 0)
	if err != nil || total != 1 || len(users) != 1 {
		t.Fatalf("ListUsers failed: total=%d, len=%d, err=%v", total, len(users), err)
	}

	batch, err := s.GetUsers(ctx, []string{"alice@example.com", "nonexistent@example.com"})
	if err != nil || len(batch) != 1 {
		t.Fatalf("GetUsers failed: len=%d, err=%v", len(batch), err)
	}

	// 6. DeleteUser
	if err := s.DeleteUser(ctx, "alice@example.com"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	_, err = s.GetUser(ctx, "alice@example.com")
	if err == nil {
		t.Fatal("expected ErrNotFound for deleted user, got nil")
	}
}

func TestUserStore_BudgetsAndGrantsWaterfall(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	userID := "bob@example.com"
	if err := s.CreateUser(ctx, &storage.UserRecord{
		ID:     userID,
		Email:  userID,
		Status: storage.StatusActive,
	}); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// 1. Initial SetBudget ($10/day)
	budget := &storage.BudgetRecord{
		UserID:     userID,
		LimitUSD:   10.0,
		PeriodType: "daily",
	}
	if err := s.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	// Verify GetBudget
	b, err := s.GetBudget(ctx, userID)
	if err != nil || b == nil {
		t.Fatalf("GetBudget failed: %v", err)
	}
	if b.LimitUSD != 10.0 || b.SpentUSD != 0.0 {
		t.Errorf("unexpected budget: %+v", b)
	}

	// 2. Pre-flight CheckBudget (should allow $5)
	check, err := s.CheckBudget(ctx, userID, 5.0)
	if err != nil || !check.Allowed {
		t.Fatalf("CheckBudget expected allowed: %+v, err=%v", check, err)
	}
	if check.BudgetUSD != 10.0 || check.RemainingUSD != 10.0 {
		t.Errorf("unexpected remaining budget: %+v", check)
	}

	// 3. Add Grant ($5 expiring tomorrow)
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	grant := &storage.GrantRecord{
		ID:        "grant-1",
		UserID:    userID,
		AmountUSD: 5.0,
		ExpiresAt: tomorrow,
		Reason:    "Hackathon prize",
	}
	if err := s.CreateGrant(ctx, grant); err != nil {
		t.Fatalf("CreateGrant failed: %v", err)
	}

	// Total budget should now be $10 (recurring) + $5 (grant) = $15
	check2, err := s.CheckBudget(ctx, userID, 12.0)
	if err != nil || !check2.Allowed {
		t.Fatalf("CheckBudget with grant expected allowed: %+v, err=%v", check2, err)
	}
	if check2.RemainingUSD != 15.0 || check2.GrantsUSD != 5.0 {
		t.Errorf("expected 15.0 total remaining, got %+v", check2)
	}

	// 4. DeductSpend $8 (absorbed fully by recurring budget)
	if err := s.DeductSpend(ctx, userID, 8.0, 1000); err != nil {
		t.Fatalf("DeductSpend failed: %v", err)
	}

	bAfter, _ := s.GetBudget(ctx, userID)
	if bAfter.SpentUSD != 8.0 || bAfter.TokensUsed != 1000 {
		t.Errorf("budget after $8 spend: %+v", bAfter)
	}

	// Grant should remain untouched (budget-first waterfall)
	gAfter, _ := s.GetGrant(ctx, userID, "grant-1")
	if gAfter.SpentUSD != 0.0 {
		t.Errorf("grant should be untouched, got spent_usd=%f", gAfter.SpentUSD)
	}

	// 5. DeductSpend $4 (recurring has $2 left, overflow $2 goes to grant)
	if err := s.DeductSpend(ctx, userID, 4.0, 500); err != nil {
		t.Fatalf("DeductSpend overflow failed: %v", err)
	}

	bAfter2, _ := s.GetBudget(ctx, userID)
	if bAfter2.SpentUSD != 10.0 {
		t.Errorf("budget should be fully spent ($10), got %f", bAfter2.SpentUSD)
	}
	gAfter2, _ := s.GetGrant(ctx, userID, "grant-1")
	if gAfter2.SpentUSD != 2.0 {
		t.Errorf("grant should have absorbed $2, got %f", gAfter2.SpentUSD)
	}

	// 6. ResetSpend should reset current period spend and unblock
	if err := s.ResetSpend(ctx, userID); err != nil {
		t.Fatalf("ResetSpend failed: %v", err)
	}
	bReset, _ := s.GetBudget(ctx, userID)
	if bReset.SpentUSD != 0.0 {
		t.Errorf("expected 0 spent after reset, got %f", bReset.SpentUSD)
	}
}

func TestUserStore_ModelLimits(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	userID := "charlie@example.com"
	_ = s.CreateUser(ctx, &storage.UserRecord{ID: userID, Email: userID})

	limit := &storage.ModelLimitRecord{
		UserID:      userID,
		ModelPrefix: "claude-opus",
		MaxDailyUSD: 25.0,
	}
	if err := s.SetModelLimit(ctx, limit); err != nil {
		t.Fatalf("SetModelLimit failed: %v", err)
	}

	limits, err := s.GetModelLimits(ctx, userID)
	if err != nil || len(limits) != 1 {
		t.Fatalf("GetModelLimits failed: len=%d, err=%v", len(limits), err)
	}
	if limits[0].ModelPrefix != "claude-opus" || limits[0].MaxDailyUSD != 25.0 {
		t.Errorf("unexpected model limit: %+v", limits[0])
	}

	if err := s.DeleteModelLimit(ctx, userID, "claude-opus"); err != nil {
		t.Fatalf("DeleteModelLimit failed: %v", err)
	}
	limitsAfter, _ := s.GetModelLimits(ctx, userID)
	if len(limitsAfter) != 0 {
		t.Errorf("expected 0 limits after delete, got %d", len(limitsAfter))
	}
}

func TestUserStore_TaskBudgets(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	tb := &storage.TaskBudget{
		TaskID:    "job-999",
		UserID:    "dave@example.com",
		LimitUSD:  1.50,
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}

	if err := s.CreateTaskBudget(ctx, tb); err != nil {
		t.Fatalf("CreateTaskBudget failed: %v", err)
	}

	// Duplicate creation fails
	if err := s.CreateTaskBudget(ctx, tb); err == nil {
		t.Fatal("expected error on duplicate task budget, got nil")
	}

	got, err := s.GetTaskBudget(ctx, "job-999")
	if err != nil || got.LimitUSD != 1.50 {
		t.Fatalf("GetTaskBudget failed: %+v, err=%v", got, err)
	}

	// CheckTaskBudget
	chk, err := s.CheckTaskBudget(ctx, "job-999", 0.50)
	if err != nil || !chk.Allowed || chk.RemainingUSD != 1.50 {
		t.Fatalf("CheckTaskBudget expected allowed: %+v, err=%v", chk, err)
	}

	// DeductTaskSpend
	if err := s.DeductTaskSpend(ctx, "job-999", 1.00); err != nil {
		t.Fatalf("DeductTaskSpend failed: %v", err)
	}

	chk2, _ := s.CheckTaskBudget(ctx, "job-999", 0.60)
	if chk2.Allowed || chk2.Reason != billing.ReasonTaskBudgetExhausted {
		t.Errorf("expected exhausted, got %+v", chk2)
	}

	// DeleteTaskBudget
	if err := s.DeleteTaskBudget(ctx, "job-999"); err != nil {
		t.Fatalf("DeleteTaskBudget failed: %v", err)
	}
	_, err = s.GetTaskBudget(ctx, "job-999")
	if err == nil {
		t.Fatal("expected ErrNotFound after deletion, got nil")
	}
}

func TestUserStore_RateLimiting(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	rateLimit := 3
	userID := "eve@example.com"
	_ = s.CreateUser(ctx, &storage.UserRecord{
		ID:        userID,
		Email:     userID,
		RateLimit: &rateLimit,
	})

	// Request 1: count=1, allowed=true
	allowed, count, limit, err := s.CheckRateLimit(ctx, userID)
	if err != nil || !allowed || count != 1 || limit != 3 {
		t.Fatalf("Rate limit check 1 failed: allowed=%v count=%d limit=%d err=%v", allowed, count, limit, err)
	}

	// Request 2: count=2, allowed=true
	allowed, count, _, _ = s.CheckRateLimit(ctx, userID)
	if !allowed || count != 2 {
		t.Fatalf("Rate limit check 2 failed: count=%d", count)
	}

	// Request 3: count=3, allowed=true
	allowed, count, _, _ = s.CheckRateLimit(ctx, userID)
	if !allowed || count != 3 {
		t.Fatalf("Rate limit check 3 failed: count=%d", count)
	}

	// Request 4: count=4, allowed=false
	allowed, count, _, _ = s.CheckRateLimit(ctx, userID)
	if allowed || count != 4 {
		t.Fatalf("Rate limit check 4 expected blocked: allowed=%v count=%d", allowed, count)
	}
}

func TestUserStore_AuditLog(t *testing.T) {
	ctx := context.Background()
	s := setupTestUserStore(t)

	userID := "frank@example.com"
	_ = s.CreateUser(ctx, &storage.UserRecord{ID: userID, Email: userID})

	entry1 := &storage.AuditRecord{
		UserID:     userID,
		ActorEmail: "admin@example.com",
		Action:     "set_budget",
		Details:    "limit_usd=50.0",
	}
	if err := s.LogAction(ctx, entry1); err != nil {
		t.Fatalf("LogAction failed: %v", err)
	}

	globalEntry := &storage.AuditRecord{
		UserID:     userID,
		ActorEmail: "admin@example.com",
		Action:     "delete_user",
		Details:    "user permanently deleted",
	}
	if err := s.LogGlobalAction(ctx, globalEntry); err != nil {
		t.Fatalf("LogGlobalAction failed: %v", err)
	}

	logs, err := s.ListAuditLog(ctx, userID, 10)
	if err != nil || len(logs) != 1 {
		t.Fatalf("ListAuditLog failed: len=%d, err=%v", len(logs), err)
	}
	if logs[0].Action != "set_budget" {
		t.Errorf("unexpected action: %s", logs[0].Action)
	}
}
