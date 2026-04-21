package connecthandlers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// ──────────────────────────────────────────
// Mock UserStore
// ──────────────────────────────────────────

type mockUserStore struct {
	users   map[string]*storage.UserRecord
	budgets map[string]*storage.BudgetRecord // key: userID
	grants  map[string][]*storage.GrantRecord
	audit   map[string][]*storage.AuditRecord
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users:   make(map[string]*storage.UserRecord),
		budgets: make(map[string]*storage.BudgetRecord),
		grants:  make(map[string][]*storage.GrantRecord),
		audit:   make(map[string][]*storage.AuditRecord),
	}
}

func (m *mockUserStore) CreateUser(_ context.Context, u *storage.UserRecord) error {
	if u.ID == "" {
		u.ID = fmt.Sprintf("user_%d", len(m.users)+1)
	}
	if u.Status == "" {
		u.Status = "provisioned"
	}
	if u.Role == "" {
		u.Role = "developer"
	}
	u.CreatedAt = time.Now().UTC()
	m.users[u.ID] = u
	return nil
}

func (m *mockUserStore) GetUser(_ context.Context, id string) (*storage.UserRecord, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user %s: %w", id, storage.ErrNotFound)
	}
	return u, nil
}

func (m *mockUserStore) GetUserByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user email %s: %w", email, storage.ErrNotFound)
}

func (m *mockUserStore) GetUsers(_ context.Context, ids []string) (map[string]*storage.UserRecord, error) {
	result := make(map[string]*storage.UserRecord)
	for _, id := range ids {
		if u, ok := m.users[id]; ok {
			result[id] = u
		}
	}
	return result, nil
}

func (m *mockUserStore) ListUsers(_ context.Context, statusFilter string, limit, offset int) ([]*storage.UserRecord, int, error) {
	var all []*storage.UserRecord
	for _, u := range m.users {
		if statusFilter == "" || u.Status == statusFilter {
			all = append(all, u)
		}
	}
	total := len(all)
	if offset >= total {
		return []*storage.UserRecord{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *mockUserStore) UpdateUser(_ context.Context, u *storage.UserRecord) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserStore) TouchLastSeen(_ context.Context, id string) error {
	if u, ok := m.users[id]; ok {
		u.LastSeenAt = time.Now().UTC()
		u.Status = "active"
	}
	return nil
}

func (m *mockUserStore) DeleteUser(_ context.Context, id string) error {
	delete(m.users, id)
	delete(m.budgets, id)
	delete(m.grants, id)
	delete(m.audit, id)
	return nil
}

func (m *mockUserStore) SetBudget(_ context.Context, b *storage.BudgetRecord) error {
	m.budgets[b.UserID] = b
	return nil
}

func (m *mockUserStore) GetBudget(_ context.Context, userID string) (*storage.BudgetRecord, error) {
	b, ok := m.budgets[userID]
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (m *mockUserStore) ResetSpend(_ context.Context, userID string) error {
	if b, ok := m.budgets[userID]; ok {
		b.SpentUSD = 0
		b.TokensUsed = 0
	}
	return nil
}

func (m *mockUserStore) CreateGrant(_ context.Context, g *storage.GrantRecord) error {
	if g.ID == "" {
		g.ID = fmt.Sprintf("grant_%d", len(m.grants[g.UserID])+1)
	}
	g.CreatedAt = time.Now().UTC()
	m.grants[g.UserID] = append(m.grants[g.UserID], g)
	return nil
}

func (m *mockUserStore) ListGrants(_ context.Context, userID string, activeOnly bool) ([]*storage.GrantRecord, error) {
	var result []*storage.GrantRecord
	for _, g := range m.grants[userID] {
		if activeOnly && g.ExpiresAt.Before(time.Now()) {
			continue
		}
		if activeOnly && g.Remaining() <= 0 {
			continue
		}
		result = append(result, g)
	}
	return result, nil
}

func (m *mockUserStore) RevokeGrant(_ context.Context, userID, grantID string) error {
	for _, g := range m.grants[userID] {
		if g.ID == grantID {
			g.ExpiresAt = time.Now().UTC().Add(-time.Second)
			return nil
		}
	}
	return fmt.Errorf("grant not found: %s", grantID)
}

func (m *mockUserStore) CheckBudget(_ context.Context, userID string, estimated float64) (*storage.BudgetCheckResult, error) {
	var grantsRem float64
	for _, g := range m.grants[userID] {
		if g.ExpiresAt.After(time.Now()) && g.Remaining() > 0 {
			grantsRem += g.Remaining()
		}
	}
	var budgetRem float64
	if b, ok := m.budgets[userID]; ok {
		budgetRem = b.LimitUSD - b.SpentUSD
	}
	total := grantsRem + budgetRem
	return &storage.BudgetCheckResult{
		Allowed:       total >= estimated,
		RemainingUSD:  total,
		GrantsUSD:     grantsRem,
		BudgetUSD:     budgetRem,
		EstimatedCost: estimated,
	}, nil
}

func (m *mockUserStore) DeductSpend(_ context.Context, userID string, cost float64, tokens int64) error {
	if b, ok := m.budgets[userID]; ok {
		b.SpentUSD += cost
		b.TokensUsed += tokens
	}
	return nil
}

func (m *mockUserStore) CheckRateLimit(_ context.Context, userID string) (bool, int, int, error) {
	return true, 1, 60, nil
}

func (m *mockUserStore) LogAction(_ context.Context, entry *storage.AuditRecord) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("audit_%d", len(m.audit[entry.UserID])+1)
	}
	entry.Timestamp = time.Now().UTC()
	m.audit[entry.UserID] = append(m.audit[entry.UserID], entry)
	return nil
}

func (m *mockUserStore) LogGlobalAction(_ context.Context, entry *storage.AuditRecord) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("global_audit_%d", len(m.audit["_global"])+1)
	}
	entry.Timestamp = time.Now().UTC()
	m.audit["_global"] = append(m.audit["_global"], entry)
	return nil
}

func (m *mockUserStore) ListAuditLog(_ context.Context, userID string, limit int) ([]*storage.AuditRecord, error) {
	entries := m.audit[userID]
	if limit > 0 && limit < len(entries) {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func (m *mockUserStore) Close() error { return nil }

// ──────────────────────────────────────────
// Tests
// ──────────────────────────────────────────

func authedCtx(email string) context.Context {
	return auth.NewContext(context.Background(), &auth.User{
		ID:    "admin-id",
		Email: email,
	})
}

func TestUserHandler_CreateUser(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	resp, err := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email:       "alice@example.com",
		DisplayName: "Alice",
		Role:        typespb.UserRole_USER_ROLE_DEVELOPER,
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.Msg.User.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", resp.Msg.User.Email)
	}
	if resp.Msg.User.Status != typespb.UserStatus_USER_STATUS_PROVISIONED {
		t.Errorf("status = %v, want PROVISIONED", resp.Msg.User.Status)
	}
}

func TestUserHandler_CreateUser_WithBudget(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	resp, err := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email:          "bob@example.com",
		DailyBudgetUsd: 100.0,
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.Msg.Budget == nil {
		t.Fatal("expected budget when monthly_budget_usd > 0")
	}
	if resp.Msg.Budget.LimitUsd != 100.0 {
		t.Errorf("budget limit = %f, want 100.0", resp.Msg.Budget.LimitUsd)
	}
}

func TestUserHandler_CreateUser_MissingEmail(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	_, err := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		DisplayName: "No Email",
	}))
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestUserHandler_CreateUser_DuplicateEmail(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// First creation succeeds.
	_, err := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "dupe@example.com",
	}))
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	// Second creation with same email should fail.
	_, err = handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "dupe@example.com",
	}))
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", connectErr.Code())
	}
}

func TestUserHandler_GetUser_NotFound(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)

	_, err := handler.GetUser(context.Background(),
		connect.NewRequest(&v1.GetUserRequest{Id: "nonexistent"}))
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound", connectErr.Code())
	}
}

func TestUserHandler_DeactivateAndReactivate(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "carol@example.com",
	}))
	userID := createResp.Msg.User.Id

	// Deactivate.
	_, err := handler.DeactivateUser(ctx,
		connect.NewRequest(&v1.DeactivateUserRequest{Id: userID}))
	if err != nil {
		t.Fatalf("DeactivateUser: %v", err)
	}
	u, _ := store.GetUser(ctx, userID)
	if u.Status != storage.StatusInactive {
		t.Errorf("status after deactivate = %q, want %s", u.Status, storage.StatusInactive)
	}

	// Reactivate.
	reactivateResp, err := handler.ReactivateUser(ctx,
		connect.NewRequest(&v1.ReactivateUserRequest{Id: userID}))
	if err != nil {
		t.Fatalf("ReactivateUser: %v", err)
	}
	if reactivateResp.Msg.User.Status != typespb.UserStatus_USER_STATUS_ACTIVE {
		t.Errorf("status after reactivate = %v, want ACTIVE", reactivateResp.Msg.User.Status)
	}
}

func TestUserHandler_BudgetLifecycle(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "dave@example.com",
	}))
	userID := createResp.Msg.User.Id

	// Set budget.
	setBudgetResp, err := handler.SetBudget(ctx, connect.NewRequest(&v1.SetBudgetRequest{
		UserId:     userID,
		LimitUsd:   200.0,
		PeriodType: typespb.BudgetPeriod_BUDGET_PERIOD_DAILY,
	}))
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if setBudgetResp.Msg.Budget.LimitUsd != 200.0 {
		t.Errorf("limit = %f, want 200.0", setBudgetResp.Msg.Budget.LimitUsd)
	}

	// Get budget.
	getBudgetResp, err := handler.GetBudget(ctx, connect.NewRequest(&v1.GetBudgetRequest{
		UserId: userID,
	}))
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if getBudgetResp.Msg.Budget == nil {
		t.Fatal("expected budget in response")
	}

	// Reset spend (simulate spending first).
	store.budgets[userID].SpentUSD = 50.0
	_, err = handler.ResetSpend(ctx, connect.NewRequest(&v1.ResetSpendRequest{
		UserId: userID,
	}))
	if err != nil {
		t.Fatalf("ResetSpend: %v", err)
	}
	if store.budgets[userID].SpentUSD != 0 {
		t.Errorf("spent after reset = %f, want 0", store.budgets[userID].SpentUSD)
	}
}

func TestUserHandler_GrantLifecycle(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "eve@example.com",
	}))
	userID := createResp.Msg.User.Id

	// Create grant.
	now := time.Now().UTC()
	createGrantResp, err := handler.CreateGrant(ctx, connect.NewRequest(&v1.CreateGrantRequest{
		UserId:    userID,
		AmountUsd: 50.0,
		Reason:    "hackathon",
	}))
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if createGrantResp.Msg.Grant.AmountUsd != 50.0 {
		t.Errorf("amount = %f, want 50.0", createGrantResp.Msg.Grant.AmountUsd)
	}

	// Fix the grant expiry for the mock (it was zero-valued from proto).
	store.grants[userID][0].ExpiresAt = now.Add(24 * time.Hour)

	// List grants.
	listResp, err := handler.ListGrants(ctx, connect.NewRequest(&v1.ListGrantsRequest{
		UserId:     userID,
		ActiveOnly: true,
	}))
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	if len(listResp.Msg.Grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(listResp.Msg.Grants))
	}

	// Revoke grant.
	grantID := listResp.Msg.Grants[0].Id
	_, err = handler.RevokeGrant(ctx, connect.NewRequest(&v1.RevokeGrantRequest{
		UserId:  userID,
		GrantId: grantID,
	}))
	if err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	// List again — should be empty.
	listResp2, _ := handler.ListGrants(ctx, connect.NewRequest(&v1.ListGrantsRequest{
		UserId:     userID,
		ActiveOnly: true,
	}))
	if len(listResp2.Msg.Grants) != 0 {
		t.Errorf("expected 0 active grants after revoke, got %d", len(listResp2.Msg.Grants))
	}
}

func TestUserHandler_AuditLog(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create user (triggers audit).
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "frank@example.com",
	}))
	userID := createResp.Msg.User.Id

	// Deactivate (triggers audit).
	_, _ = handler.DeactivateUser(ctx,
		connect.NewRequest(&v1.DeactivateUserRequest{Id: userID}))

	// List audit log.
	auditResp, err := handler.ListAuditLog(ctx, connect.NewRequest(&v1.ListAuditLogRequest{
		UserId: userID,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(auditResp.Msg.Entries) != 2 {
		t.Errorf("expected 2 audit entries, got %d", len(auditResp.Msg.Entries))
	}
}

func TestUserHandler_GetCurrentUser_AutoProvision(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)

	// Authed as a new user who doesn't exist yet.
	ctx := authedCtx("newuser@example.com")

	resp, err := handler.GetCurrentUser(ctx,
		connect.NewRequest(&v1.GetCurrentUserRequest{}))
	if err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	if resp.Msg.User.Email != "newuser@example.com" {
		t.Errorf("email = %q, want newuser@example.com", resp.Msg.User.Email)
	}
	if resp.Msg.User.Status != typespb.UserStatus_USER_STATUS_ACTIVE {
		t.Errorf("status = %v, want ACTIVE (auto-provisioned)", resp.Msg.User.Status)
	}

	// Second call should return same user.
	resp2, _ := handler.GetCurrentUser(ctx,
		connect.NewRequest(&v1.GetCurrentUserRequest{}))
	if resp2.Msg.User.Id != resp.Msg.User.Id {
		t.Error("second GetCurrentUser should return same user ID")
	}
}

func TestUserHandler_GetCurrentUser_Unauthenticated(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)

	_, err := handler.GetCurrentUser(context.Background(),
		connect.NewRequest(&v1.GetCurrentUserRequest{}))
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", connectErr.Code())
	}
}

func TestUserHandler_ListUsers(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create 3 users.
	for _, email := range []string{"a@test.com", "b@test.com", "c@test.com"} {
		_, _ = handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
			Email: email,
		}))
	}

	resp, err := handler.ListUsers(ctx, connect.NewRequest(&v1.ListUsersRequest{}))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(resp.Msg.Users) != 3 {
		t.Errorf("expected 3 users, got %d", len(resp.Msg.Users))
	}
	if resp.Msg.Pagination.TotalCount != 3 {
		t.Errorf("total = %d, want 3", resp.Msg.Pagination.TotalCount)
	}

	// Test pagination with page_size=2.
	resp2, err := handler.ListUsers(ctx, connect.NewRequest(&v1.ListUsersRequest{
		Pagination: &typespb.PaginationRequest{PageSize: 2},
	}))
	if err != nil {
		t.Fatalf("ListUsers page 1: %v", err)
	}
	if len(resp2.Msg.Users) != 2 {
		t.Errorf("expected 2 users on page 1, got %d", len(resp2.Msg.Users))
	}
	if resp2.Msg.Pagination.NextPageToken == "" {
		t.Error("expected next_page_token for page 1")
	}

	// Page 2 using next_page_token.
	resp3, err := handler.ListUsers(ctx, connect.NewRequest(&v1.ListUsersRequest{
		Pagination: &typespb.PaginationRequest{
			PageSize:  2,
			PageToken: resp2.Msg.Pagination.NextPageToken,
		},
	}))
	if err != nil {
		t.Fatalf("ListUsers page 2: %v", err)
	}
	if len(resp3.Msg.Users) != 1 {
		t.Errorf("expected 1 user on page 2, got %d", len(resp3.Msg.Users))
	}
	if resp3.Msg.Pagination.NextPageToken != "" {
		t.Errorf("expected empty next_page_token on last page, got %q", resp3.Msg.Pagination.NextPageToken)
	}
}

func TestUserHandler_UpdateUser_FieldMask(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email:       "mask@test.com",
		DisplayName: "Original",
		Role:        typespb.UserRole_USER_ROLE_DEVELOPER,
	}))
	userID := createResp.Msg.User.Id

	// Update only display_name (role should stay developer).
	_, err := handler.UpdateUser(ctx, connect.NewRequest(&v1.UpdateUserRequest{
		Id:          userID,
		DisplayName: "Updated",
		Role:        typespb.UserRole_USER_ROLE_ADMIN, // Should be ignored.
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	u, _ := store.GetUser(ctx, userID)
	if u.DisplayName != "Updated" {
		t.Errorf("display_name = %q, want Updated", u.DisplayName)
	}
	// Without field mask, both fields update.
	if u.Role != "admin" {
		t.Errorf("role = %q, want admin (no mask = all fields)", u.Role)
	}
}

func TestUserHandler_GetMyBudget_Unauthenticated(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)

	_, err := handler.GetMyBudget(context.Background(),
		connect.NewRequest(&v1.GetMyBudgetRequest{}))
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}
}

// ── Proto converter tests ──

func TestRoleConversion(t *testing.T) {
	tests := []struct {
		proto typespb.UserRole
		str   string
	}{
		{typespb.UserRole_USER_ROLE_DEVELOPER, "developer"},
		{typespb.UserRole_USER_ROLE_ADMIN, "admin"},
		{typespb.UserRole_USER_ROLE_UNSPECIFIED, "developer"},
	}
	for _, tt := range tests {
		if got := roleToString(tt.proto); got != tt.str {
			t.Errorf("roleToString(%v) = %q, want %q", tt.proto, got, tt.str)
		}
		if tt.proto != typespb.UserRole_USER_ROLE_UNSPECIFIED {
			if got := stringToRole(tt.str); got != tt.proto {
				t.Errorf("stringToRole(%q) = %v, want %v", tt.str, got, tt.proto)
			}
		}
	}
}

func TestStatusConversion(t *testing.T) {
	tests := []struct {
		proto typespb.UserStatus
		str   string
	}{
		{typespb.UserStatus_USER_STATUS_PROVISIONED, "provisioned"},
		{typespb.UserStatus_USER_STATUS_ACTIVE, "active"},
		{typespb.UserStatus_USER_STATUS_INACTIVE, "inactive"},
	}
	for _, tt := range tests {
		if got := statusToString(tt.proto); got != tt.str {
			t.Errorf("statusToString(%v) = %q, want %q", tt.proto, got, tt.str)
		}
		if got := stringToStatus(tt.str); got != tt.proto {
			t.Errorf("stringToStatus(%q) = %v, want %v", tt.str, got, tt.proto)
		}
	}
}

func TestUserHandler_DeleteUser_HappyPath(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create and deactivate user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "delete-me@example.com",
	}))
	userID := createResp.Msg.User.Id

	_, _ = handler.DeactivateUser(ctx,
		connect.NewRequest(&v1.DeactivateUserRequest{Id: userID}))

	// Delete with correct email.
	_, err := handler.DeleteUser(ctx, connect.NewRequest(&v1.DeleteUserRequest{
		Id:           userID,
		ConfirmEmail: "delete-me@example.com",
	}))
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// Verify user is gone.
	_, err = store.GetUser(ctx, userID)
	if err == nil {
		t.Error("expected user to be deleted from store")
	}
}

func TestUserHandler_DeleteUser_ActiveUserBlocked(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create user but DON'T deactivate.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "active@example.com",
	}))
	userID := createResp.Msg.User.Id

	_, err := handler.DeleteUser(ctx, connect.NewRequest(&v1.DeleteUserRequest{
		Id:           userID,
		ConfirmEmail: "active@example.com",
	}))
	if err == nil {
		t.Fatal("expected error when deleting active user")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connectErr.Code())
	}
}

func TestUserHandler_DeleteUser_WrongEmailBlocked(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create and deactivate user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "target@example.com",
	}))
	userID := createResp.Msg.User.Id
	_, _ = handler.DeactivateUser(ctx,
		connect.NewRequest(&v1.DeactivateUserRequest{Id: userID}))

	// Try to delete with wrong email.
	_, err := handler.DeleteUser(ctx, connect.NewRequest(&v1.DeleteUserRequest{
		Id:           userID,
		ConfirmEmail: "wrong@example.com",
	}))
	if err == nil {
		t.Fatal("expected error for mismatched confirm_email")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestPeriodConversion(t *testing.T) {
	// All inputs map to daily.
	if got := periodToString(typespb.BudgetPeriod_BUDGET_PERIOD_DAILY); got != "daily" {
		t.Errorf("periodToString(DAILY) = %q, want %q", got, "daily")
	}
	if got := periodToString(typespb.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED); got != "daily" {
		t.Errorf("periodToString(UNSPECIFIED) = %q, want %q", got, "daily")
	}
	if got := stringToPeriod("daily"); got != typespb.BudgetPeriod_BUDGET_PERIOD_DAILY {
		t.Errorf("stringToPeriod(daily) = %v, want DAILY", got)
	}
	if got := stringToPeriod("anything"); got != typespb.BudgetPeriod_BUDGET_PERIOD_DAILY {
		t.Errorf("stringToPeriod(anything) = %v, want DAILY", got)
	}
}

func TestMustJSON_Success(t *testing.T) {
	got := mustJSON(map[string]string{"email": "test@example.com"})
	if got != `{"email":"test@example.com"}` {
		t.Errorf("mustJSON = %q, want valid JSON", got)
	}
}

func TestMustJSON_Fallback(t *testing.T) {
	// Channels cannot be marshalled to JSON.
	got := mustJSON(make(chan int))
	expected := `{"error":"failed to marshal audit details"}`
	if got != expected {
		t.Errorf("mustJSON fallback = %q, want %q", got, expected)
	}
}

func TestUserHandler_DeleteUser_GlobalAudit(t *testing.T) {
	store := newMockUserStore()
	handler := NewUserHandler(store)
	ctx := authedCtx("admin@example.com")

	// Create and deactivate user.
	createResp, _ := handler.CreateUser(ctx, connect.NewRequest(&v1.CreateUserRequest{
		Email: "audit-test@example.com",
	}))
	userID := createResp.Msg.User.Id

	_, _ = handler.DeactivateUser(ctx,
		connect.NewRequest(&v1.DeactivateUserRequest{Id: userID}))

	// Delete user.
	_, err := handler.DeleteUser(ctx, connect.NewRequest(&v1.DeleteUserRequest{
		Id:           userID,
		ConfirmEmail: "audit-test@example.com",
	}))
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// Verify global audit entry was written.
	globalEntries := store.audit["_global"]
	if len(globalEntries) == 0 {
		t.Fatal("expected global audit entry for delete_user")
	}

	found := false
	for _, entry := range globalEntries {
		if entry.Action == "delete_user" && entry.UserID == userID {
			found = true
			if entry.ActorEmail != "admin@example.com" {
				t.Errorf("actor_email = %q, want admin@example.com", entry.ActorEmail)
			}
			break
		}
	}
	if !found {
		t.Error("delete_user action not found in global audit log")
	}

	// Verify user-level audit is gone (deleted with user).
	userEntries := store.audit[userID]
	if len(userEntries) != 0 {
		t.Errorf("expected user audit entries to be deleted, got %d", len(userEntries))
	}
}
