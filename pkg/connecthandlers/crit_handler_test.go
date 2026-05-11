package connecthandlers_test

// Integration tests for CRIT-16, CRIT-18, CRIT-19 handler fixes.
// Uses the integrationStore mock from integration_test.go.

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
)

// ─── CRIT-18: GetUser returns CodeNotFound for missing users, not internal ────

func TestGetUser_NotFound_ReturnsCodeNotFound(t *testing.T) {
	store := newIntegrationStore()
	// Seed admin so middleware can resolve caller identity.
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.GetUser(context.Background(), connect.NewRequest(&v1.GetUserRequest{
		Id: "does-not-exist",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != connect.CodeNotFound {
		t.Errorf("error code = %v, want CodeNotFound", ce.Code())
	}
}

// ─── CRIT-18: UpdateUser for missing user returns CodeNotFound ────────────────

func TestUpdateUser_NotFound_ReturnsCodeNotFound(t *testing.T) {
	store := newIntegrationStore()
	// Seed admin so auth check passes.
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.UpdateUser(context.Background(), connect.NewRequest(&v1.UpdateUserRequest{
		Id:          "ghost-user",
		DisplayName: "Doesn't Matter",
	}))
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
	ce, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if ce.Code() != connect.CodeNotFound {
		t.Errorf("error code = %v, want CodeNotFound", ce.Code())
	}
}

// ─── CRIT-19: ListUsers with large page_token returns CodeInvalidArgument ─────

func TestListUsers_LargePageToken_ReturnsInvalidArgument(t *testing.T) {
	store := newIntegrationStore()
	// Seed admin so the admin-scope middleware can resolve the caller.
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.ListUsers(context.Background(), connect.NewRequest(&v1.ListUsersRequest{
		Pagination: &typespb.PaginationRequest{
			PageSize:  10,
			PageToken: "999999999",
		},
	}))
	if err == nil {
		t.Fatal("expected error for oversized page_token, got nil")
	}
	ce, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want CodeInvalidArgument", ce.Code())
	}
}
