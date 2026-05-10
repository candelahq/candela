package connecthandlers_test

// Integration tests for pointer-field semantics introduced in the billing-
// hardening push: UpdateUser with nil DisplayName should not clear it, nil
// RateLimit should not reset it, etc.

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ─── UpdateUser — pointer field isolation ────────────────────────────────────

func TestIntegration_UpdateUser_DisplayNameOnly_PreservesRateLimit(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	rateLimit := 50
	store.users["target"] = &storage.UserRecord{
		ID:          "target",
		Email:       "target@test.com",
		Role:        "developer",
		Status:      storage.StatusActive,
		DisplayName: func(s string) *string { return &s }("Old Name"),
		RateLimit:   &rateLimit,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	// Update only display_name via field mask
	_, err := client.UpdateUser(context.Background(), connect.NewRequest(&v1.UpdateUserRequest{
		Id:          "target",
		DisplayName: "New Name",
		UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	u := store.users["target"]
	if u.DisplayName == nil || *u.DisplayName != "New Name" {
		t.Errorf("DisplayName = %v, want &New Name", u.DisplayName)
	}
	// RateLimit must be untouched
	if u.RateLimit == nil || *u.RateLimit != 50 {
		t.Errorf("RateLimit = %v, want &50 (must be unchanged)", u.RateLimit)
	}
}

func TestIntegration_UpdateUser_RoleOnly_PreservesDisplayName(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	store.users["target"] = &storage.UserRecord{
		ID:          "target",
		Email:       "target@test.com",
		Role:        "developer",
		Status:      storage.StatusActive,
		DisplayName: func(s string) *string { return &s }("Keep Me"),
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	_, err := client.UpdateUser(context.Background(), connect.NewRequest(&v1.UpdateUserRequest{
		Id:         "target",
		Role:       typespb.UserRole_USER_ROLE_ADMIN,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"role"}},
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	u := store.users["target"]
	if u.Role != "admin" {
		t.Errorf("Role = %q, want admin", u.Role)
	}
	// DisplayName must be untouched
	if u.DisplayName == nil || *u.DisplayName != "Keep Me" {
		t.Errorf("DisplayName = %v, want &Keep Me (must be unchanged)", u.DisplayName)
	}
}

func TestIntegration_CreateUser_DisplayName_RoundTrips(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	resp, err := client.CreateUser(context.Background(), connect.NewRequest(&v1.CreateUserRequest{
		Email:       "newuser@test.com",
		DisplayName: "New User",
		Role:        typespb.UserRole_USER_ROLE_DEVELOPER,
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.Msg.User.DisplayName != "New User" {
		t.Errorf("DisplayName = %q, want New User", resp.Msg.User.DisplayName)
	}

	// Verify it's stored as *string in the backing store
	var created *storage.UserRecord
	for _, u := range store.users {
		if u.Email == "newuser@test.com" {
			created = u
			break
		}
	}
	if created == nil {
		t.Fatal("user not found in store")
	}
	if created.DisplayName == nil || *created.DisplayName != "New User" {
		t.Errorf("stored DisplayName = %v, want &New User", created.DisplayName)
	}
}

func TestIntegration_UpdateUser_NoFieldMask_UpdatesAll(t *testing.T) {
	store := newIntegrationStore()
	store.users["admin1"] = &storage.UserRecord{
		ID: "admin1", Email: "admin@test.com", Role: "admin", Status: storage.StatusActive,
	}
	store.users["target"] = &storage.UserRecord{
		ID:     "target",
		Email:  "target@test.com",
		Role:   "developer",
		Status: storage.StatusActive,
	}
	client := startTestServerWithClient(t, store, "admin@test.com")

	resp, err := client.UpdateUser(context.Background(), connect.NewRequest(&v1.UpdateUserRequest{
		Id:          "target",
		DisplayName: "Patched",
		Role:        typespb.UserRole_USER_ROLE_ADMIN,
		// No UpdateMask → all fields
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if resp.Msg.User.Role != typespb.UserRole_USER_ROLE_ADMIN {
		t.Errorf("Role = %v, want ADMIN", resp.Msg.User.Role)
	}
	if resp.Msg.User.DisplayName != "Patched" {
		t.Errorf("DisplayName = %q, want Patched", resp.Msg.User.DisplayName)
	}
}
