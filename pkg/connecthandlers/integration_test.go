package connecthandlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"connectrpc.com/validate"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ──────────────────────────────────────────
// Mock UserStore for integration tests
// ──────────────────────────────────────────

type integrationStore struct {
	users   map[string]*storage.UserRecord
	budgets map[string]*storage.BudgetRecord
	grants  map[string][]*storage.GrantRecord
	audit   map[string][]*storage.AuditRecord
}

func newIntegrationStore() *integrationStore {
	return &integrationStore{
		users:   make(map[string]*storage.UserRecord),
		budgets: make(map[string]*storage.BudgetRecord),
		grants:  make(map[string][]*storage.GrantRecord),
		audit:   make(map[string][]*storage.AuditRecord),
	}
}

func (s *integrationStore) CreateUser(_ context.Context, u *storage.UserRecord) error {
	if u.ID == "" {
		u.ID = fmt.Sprintf("user_%d", len(s.users)+1)
	}
	if u.Status == "" {
		u.Status = storage.StatusProvisioned
	}
	if u.Role == "" {
		u.Role = "developer"
	}
	u.CreatedAt = time.Now().UTC()
	s.users[u.ID] = u
	return nil
}

func (s *integrationStore) GetUser(_ context.Context, id string) (*storage.UserRecord, error) {
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user %s: %w", id, storage.ErrNotFound)
	}
	return u, nil
}

func (s *integrationStore) GetUserByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	for _, u := range s.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user %s: %w", email, storage.ErrNotFound)
}

func (s *integrationStore) ListUsers(_ context.Context, statusFilter string, limit, offset int) ([]*storage.UserRecord, int, error) {
	var result []*storage.UserRecord
	for _, u := range s.users {
		if statusFilter == "" || u.Status == statusFilter {
			result = append(result, u)
		}
	}
	return result, len(result), nil
}

func (s *integrationStore) UpdateUser(_ context.Context, u *storage.UserRecord) error {
	s.users[u.ID] = u
	return nil
}

func (s *integrationStore) TouchLastSeen(_ context.Context, id string) error {
	if u, ok := s.users[id]; ok {
		u.LastSeenAt = time.Now().UTC()
	}
	return nil
}

func (s *integrationStore) SetBudget(_ context.Context, b *storage.BudgetRecord) error {
	s.budgets[b.UserID] = b
	return nil
}

func (s *integrationStore) GetBudget(_ context.Context, userID string) (*storage.BudgetRecord, error) {
	b, ok := s.budgets[userID]
	if !ok {
		return nil, fmt.Errorf("budget %s: %w", userID, storage.ErrNotFound)
	}
	return b, nil
}

func (s *integrationStore) ResetSpend(_ context.Context, userID string) error {
	if b, ok := s.budgets[userID]; ok {
		b.SpentUSD = 0
	}
	return nil
}

func (s *integrationStore) CreateGrant(_ context.Context, g *storage.GrantRecord) error {
	g.ID = fmt.Sprintf("grant_%d", len(s.grants[g.UserID])+1)
	s.grants[g.UserID] = append(s.grants[g.UserID], g)
	return nil
}

func (s *integrationStore) ListGrants(_ context.Context, userID string, activeOnly bool) ([]*storage.GrantRecord, error) {
	return s.grants[userID], nil
}

func (s *integrationStore) RevokeGrant(_ context.Context, userID, grantID string) error {
	grants := s.grants[userID]
	for _, g := range grants {
		if g.ID == grantID {
			g.ExpiresAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("grant %s: %w", grantID, storage.ErrNotFound)
}

func (s *integrationStore) CheckBudget(_ context.Context, userID string, estimatedCostUSD float64) (*storage.BudgetCheckResult, error) {
	return &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100}, nil
}

func (s *integrationStore) DeductSpend(_ context.Context, userID string, costUSD float64, tokens int64) error {
	return nil
}

func (s *integrationStore) CheckRateLimit(_ context.Context, userID string) (bool, int, int, error) {
	return true, 0, 100, nil
}

func (s *integrationStore) LogAction(_ context.Context, a *storage.AuditRecord) error {
	a.ID = fmt.Sprintf("audit_%d", len(s.audit[a.UserID])+1)
	s.audit[a.UserID] = append(s.audit[a.UserID], a)
	return nil
}

func (s *integrationStore) ListAuditLog(_ context.Context, userID string, limit int) ([]*storage.AuditRecord, error) {
	return s.audit[userID], nil
}

func (s *integrationStore) Close() error { return nil }

// ──────────────────────────────────────────
// Test server helpers
// ──────────────────────────────────────────

// testAuthHeader is used by tests to inject identity via HTTP header.
const testAuthHeader = "X-Test-Email"

// testAuthMiddleware is a simple HTTP middleware that reads X-Test-Email and
// injects an auth.User into the request context (mirroring IAPMiddleware).
func testAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := r.Header.Get(testAuthHeader)
		if email != "" {
			user := &auth.User{
				ID:    "test-" + email,
				Email: email,
			}
			r = r.WithContext(auth.NewContext(r.Context(), user))
		}
		next.ServeHTTP(w, r)
	})
}

// startTestServer creates a real HTTP test server with auth middleware,
// protovalidate, and admin guard interceptors.
func startTestServer(t *testing.T, store *integrationStore) candelav1connect.UserServiceClient {
	t.Helper()

	validateInterceptor := validate.NewInterceptor()
	adminInterceptor := auth.AdminInterceptor(store)

	mux := http.NewServeMux()
	path, handler := candelav1connect.NewUserServiceHandler(
		connecthandlers.NewUserHandler(store),
		connect.WithInterceptors(validateInterceptor, adminInterceptor),
	)
	mux.Handle(path, handler)

	// Wrap with test auth middleware.
	server := httptest.NewServer(testAuthMiddleware(mux))
	t.Cleanup(server.Close)

	client := candelav1connect.NewUserServiceClient(
		http.DefaultClient,
		server.URL,
	)

	return client
}

// testAuthInterceptor injects X-Test-Email into outgoing requests.
type testAuthInterceptor struct {
	email string
}

func (i *testAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if i.email != "" {
			req.Header().Set(testAuthHeader, i.email)
		}
		return next(ctx, req)
	}
}

func (i *testAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *testAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// startTestServerWithClient creates a server and a client authenticated as the given email.
func startTestServerWithClient(t *testing.T, store *integrationStore, email string) candelav1connect.UserServiceClient {
	t.Helper()

	validateInterceptor := validate.NewInterceptor()
	adminInterceptor := auth.AdminInterceptor(store)

	mux := http.NewServeMux()
	path, handler := candelav1connect.NewUserServiceHandler(
		connecthandlers.NewUserHandler(store),
		connect.WithInterceptors(validateInterceptor, adminInterceptor),
	)
	mux.Handle(path, handler)

	server := httptest.NewServer(testAuthMiddleware(mux))
	t.Cleanup(server.Close)

	client := candelav1connect.NewUserServiceClient(
		http.DefaultClient,
		server.URL,
		connect.WithInterceptors(&testAuthInterceptor{email: email}),
	)

	return client
}

// ──────────────────────────────────────────
// Protovalidate Integration Tests
// ──────────────────────────────────────────

func TestIntegration_Validate_CreateUser_InvalidEmail(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.CreateUser(context.Background(), connect.NewRequest(&v1.CreateUserRequest{
		Email: "not-an-email",
	}))
	if err == nil {
		t.Fatal("expected validation error for invalid email")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
	t.Logf("validation error: %s", connectErr.Message())
}

func TestIntegration_Validate_CreateUser_NegativeBudget(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.CreateUser(context.Background(), connect.NewRequest(&v1.CreateUserRequest{
		Email:            "new@test.com",
		MonthlyBudgetUsd: -10.0,
	}))
	if err == nil {
		t.Fatal("expected validation error for negative budget")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestIntegration_Validate_SetBudget_ZeroLimit(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.SetBudget(context.Background(), connect.NewRequest(&v1.SetBudgetRequest{
		UserId:   "admin1",
		LimitUsd: 0,
	}))
	if err == nil {
		t.Fatal("expected validation error for zero budget limit")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestIntegration_Validate_GetUser_EmptyID(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.GetUser(context.Background(), connect.NewRequest(&v1.GetUserRequest{
		Id: "",
	}))
	if err == nil {
		t.Fatal("expected validation error for empty user ID")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestIntegration_Validate_CreateGrant_ZeroAmount(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.CreateGrant(context.Background(), connect.NewRequest(&v1.CreateGrantRequest{
		UserId:    "admin1",
		AmountUsd: 0,
		Reason:    "test",
	}))
	if err == nil {
		t.Fatal("expected validation error for zero grant amount")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestIntegration_Validate_AuditLog_LimitTooHigh(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.ListAuditLog(context.Background(), connect.NewRequest(&v1.ListAuditLogRequest{
		UserId: "admin1",
		Limit:  999,
	}))
	if err == nil {
		t.Fatal("expected validation error for limit > 500")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestIntegration_Validate_ValidRequest_Passes(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	resp, err := client.CreateUser(context.Background(), connect.NewRequest(&v1.CreateUserRequest{
		Email:       "valid@example.com",
		DisplayName: "Valid User",
		Role:        typespb.UserRole_USER_ROLE_DEVELOPER,
	}))
	if err != nil {
		t.Fatalf("valid request should succeed: %v", err)
	}
	if resp.Msg.User.Email != "valid@example.com" {
		t.Errorf("email = %q, want valid@example.com", resp.Msg.User.Email)
	}
}

// ──────────────────────────────────────────
// Admin Guard Integration Tests
// ──────────────────────────────────────────

func TestIntegration_AdminGuard_DeveloperBlocked(t *testing.T) {
	store := newIntegrationStore()
	store.users["test-dev@test.com"] = &storage.UserRecord{
		ID: "test-dev@test.com", Email: "dev@test.com", Role: "developer", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "dev@test.com")

	_, err := client.ListUsers(context.Background(), connect.NewRequest(&v1.ListUsersRequest{}))
	if err == nil {
		t.Fatal("expected PermissionDenied for developer calling ListUsers")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", connectErr.Code())
	}
}

func TestIntegration_AdminGuard_UnauthenticatedBlocked(t *testing.T) {
	store := newIntegrationStore()
	// Use startTestServer (no auth header) for unauthenticated test.
	client := startTestServer(t, store)

	_, err := client.CreateUser(context.Background(), connect.NewRequest(&v1.CreateUserRequest{
		Email: "test@example.com",
	}))
	if err == nil {
		t.Fatal("expected Unauthenticated error")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", connectErr.Code())
	}
}

func TestIntegration_AdminGuard_SelfServiceAllowed(t *testing.T) {
	store := newIntegrationStore()
	store.users["test-dev@test.com"] = &storage.UserRecord{
		ID: "test-dev@test.com", Email: "dev@test.com", Role: "developer", Status: storage.StatusActive,
	}
	store.budgets["test-dev@test.com"] = &storage.BudgetRecord{
		UserID: "test-dev@test.com", LimitUSD: 100, PeriodType: "monthly",
	}
	client := startTestServerWithClient(t, store, "dev@test.com")

	resp, err := client.GetCurrentUser(context.Background(), connect.NewRequest(&v1.GetCurrentUserRequest{}))
	if err != nil {
		t.Fatalf("GetCurrentUser should be allowed for developers: %v", err)
	}
	if resp.Msg.User.Email != "dev@test.com" {
		t.Errorf("email = %q, want dev@test.com", resp.Msg.User.Email)
	}
}

func TestIntegration_AdminGuard_AdminAllowed(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	resp, err := client.ListUsers(context.Background(), connect.NewRequest(&v1.ListUsersRequest{}))
	if err != nil {
		t.Fatalf("admin should be able to call ListUsers: %v", err)
	}
	if len(resp.Msg.Users) != 1 {
		t.Errorf("users = %d, want 1", len(resp.Msg.Users))
	}
}

// ──────────────────────────────────────────
// Handler Edge Case Tests
// ──────────────────────────────────────────

func TestIntegration_UpdateUser_EmptyFieldMask(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	store.users["target"] = &storage.UserRecord{
		ID: "target", Email: "target@test.com", Role: "developer",
		DisplayName: "Original", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	resp, err := client.UpdateUser(context.Background(), connect.NewRequest(&v1.UpdateUserRequest{
		Id:          "target",
		DisplayName: "Updated",
		Role:        typespb.UserRole_USER_ROLE_ADMIN,
	}))
	if err != nil {
		t.Fatalf("UpdateUser with no mask should succeed: %v", err)
	}
	if resp.Msg.User.DisplayName != "Updated" {
		t.Errorf("display_name = %q, want Updated", resp.Msg.User.DisplayName)
	}
}

func TestIntegration_Validate_GrantExpiresBeforeStarts(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	now := time.Now()
	past := now.Add(-24 * time.Hour)

	_, err := client.CreateGrant(context.Background(), connect.NewRequest(&v1.CreateGrantRequest{
		UserId:    "admin1",
		AmountUsd: 50.0,
		Reason:    "test grant",
		StartsAt:  timestamppb.New(now),
		ExpiresAt: timestamppb.New(past),
	}))
	if err == nil {
		t.Fatal("expected validation error for expires_at < starts_at")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connectErr.Code())
	}
	t.Logf("CEL validation error: %s", connectErr.Message())
}
