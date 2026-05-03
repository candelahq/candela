package connecthandlers

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ── Integration tests for audit v2 fixes ──

// mockUserStore implements the minimal UserStore interface for testing.
type mockUserStoreV2 struct {
	storage.UserStore
	users   map[string]*storage.UserRecord
	budgets map[string]*storage.BudgetRecord
	grants  []*storage.GrantRecord
}

func newMockUserStoreV2() *mockUserStoreV2 {
	return &mockUserStoreV2{
		users:   make(map[string]*storage.UserRecord),
		budgets: make(map[string]*storage.BudgetRecord),
	}
}

func (m *mockUserStoreV2) GetUser(_ context.Context, id string) (*storage.UserRecord, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockUserStoreV2) GetUserByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (m *mockUserStoreV2) UpdateUser(_ context.Context, u *storage.UserRecord) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserStoreV2) SetBudget(_ context.Context, b *storage.BudgetRecord) error {
	m.budgets[b.UserID] = b
	return nil
}

func (m *mockUserStoreV2) CreateGrant(_ context.Context, g *storage.GrantRecord) error {
	m.grants = append(m.grants, g)
	return nil
}

func (m *mockUserStoreV2) LogAction(_ context.Context, _ *storage.AuditRecord) error {
	return nil
}

func (m *mockUserStoreV2) ListAuditLog(_ context.Context, _ string, limit int) ([]*storage.AuditRecord, error) {
	entries := make([]*storage.AuditRecord, limit)
	for i := range entries {
		entries[i] = &storage.AuditRecord{ID: "audit-" + string(rune(i))}
	}
	return entries, nil
}

// TestSetBudget_RejectsNegativeLimit verifies H14 fix.
func TestSetBudget_RejectsNegativeLimit(t *testing.T) {
	store := newMockUserStoreV2()
	handler := NewUserHandler(store, 10.0)

	_, err := handler.SetBudget(context.Background(), connect.NewRequest(&v1.SetBudgetRequest{
		UserId:   "user-1",
		LimitUsd: -5.0,
	}))
	if err == nil {
		t.Fatal("expected error for negative budget limit")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// TestSetBudget_RejectsZeroLimit verifies H14 fix.
func TestSetBudget_RejectsZeroLimit(t *testing.T) {
	store := newMockUserStoreV2()
	handler := NewUserHandler(store, 10.0)

	_, err := handler.SetBudget(context.Background(), connect.NewRequest(&v1.SetBudgetRequest{
		UserId:   "user-1",
		LimitUsd: 0,
	}))
	if err == nil {
		t.Fatal("expected error for zero budget limit")
	}
}

// TestCreateGrant_RejectsInvalidDates verifies H15 fix.
func TestCreateGrant_RejectsInvalidDates(t *testing.T) {
	store := newMockUserStoreV2()
	handler := NewUserHandler(store, 10.0)

	now := time.Now()
	// ExpiresAt before StartsAt.
	_, err := handler.CreateGrant(context.Background(), connect.NewRequest(&v1.CreateGrantRequest{
		UserId:    "user-1",
		AmountUsd: 50.0,
		Reason:    "test",
		StartsAt:  timestamppb.New(now),
		ExpiresAt: timestamppb.New(now.Add(-1 * time.Hour)),
	}))
	if err == nil {
		t.Fatal("expected error when expires_at < starts_at")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// TestCreateGrant_RejectsNilTimestamps verifies H15 fix.
func TestCreateGrant_RejectsNilTimestamps(t *testing.T) {
	store := newMockUserStoreV2()
	handler := NewUserHandler(store, 10.0)

	_, err := handler.CreateGrant(context.Background(), connect.NewRequest(&v1.CreateGrantRequest{
		UserId:    "user-1",
		AmountUsd: 50.0,
		Reason:    "test",
		// StartsAt and ExpiresAt are nil.
	}))
	if err == nil {
		t.Fatal("expected error for nil timestamps")
	}
}

// TestUpdateUser_DeveloperCannotEscalateRole verifies H16 fix.
func TestUpdateUser_DeveloperCannotEscalateRole(t *testing.T) {
	store := newMockUserStoreV2()
	// Add a developer user and the caller (also a developer).
	store.users["target-user"] = &storage.UserRecord{
		ID:    "target-user",
		Email: "target@example.com",
		Role:  "developer",
	}
	store.users["caller-user"] = &storage.UserRecord{
		ID:    "caller-user",
		Email: "caller@example.com",
		Role:  "developer", // NOT admin
	}

	handler := NewUserHandler(store, 10.0)

	// Inject caller into context.
	ctx := auth.NewContext(context.Background(), &auth.User{
		ID:    "caller-user",
		Email: "caller@example.com",
	})

	_, err := handler.UpdateUser(ctx, connect.NewRequest(&v1.UpdateUserRequest{
		Id:   "target-user",
		Role: typespb.UserRole_USER_ROLE_ADMIN,
	}))
	if err == nil {
		t.Fatal("expected error when non-admin tries to change role")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
	}
}

// TestListAuditLog_ClampsLimit verifies H13 fix.
func TestListAuditLog_ClampsLimit(t *testing.T) {
	store := newMockUserStoreV2()
	handler := NewUserHandler(store, 10.0)

	// Request with limit=0 should default to 50.
	resp, err := handler.ListAuditLog(context.Background(), connect.NewRequest(&v1.ListAuditLogRequest{
		UserId: "user-1",
		Limit:  0,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Entries) != 50 {
		t.Errorf("expected 50 entries (clamped default), got %d", len(resp.Msg.Entries))
	}
}
