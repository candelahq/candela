package firestoredb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Alice@Example.Com", "alice@example.com"},
		{"user/test@example.com", "user_test@example.com"},
		{"USER/TEST@example.com", "user_test@example.com"},
		{"no_slash@example.com", "no_slash@example.com"},
	}

	for _, tt := range tests {
		got := sanitizeID(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestUpdateUser_PartialUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := fmt.Sprintf("partial-%d@example.com", time.Now().UnixNano())
	sanitizedID := sanitizeID(userID)

	user := &storage.UserRecord{
		Email:       userID,
		DisplayName: "Original Name",
		Role:        "admin",
		Status:      "active",
	}
	t.Cleanup(func() { cleanupUser(ctx, s, sanitizedID) })

	// 1. Create the user
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 2. Perform a partial update (only changing DisplayName)
	// We create a new struct with only ID and DisplayName.
	// Because of 'omitempty' tags, Role and Status should NOT be overwritten in Firestore.
	update := &storage.UserRecord{
		ID:          user.ID, // Need ID to know which doc to update
		DisplayName: "Updated Name",
	}

	if err := s.UpdateUser(ctx, update); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	// 3. Verify that Role and Status were preserved
	got, err := s.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	if got.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Updated Name")
	}
	if got.Role != "admin" {
		t.Errorf("Role was overwritten! got %q, want %q", got.Role, "admin")
	}
	if got.Status != "active" {
		t.Errorf("Status was overwritten! got %q, want %q", got.Status, "active")
	}
}
