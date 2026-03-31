package projectdb

import (
	"context"
	"os"
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	path := t.TempDir() + "/test.db"
	store, err := New(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		os.Remove(path)
	})
	return store
}

func TestProjectCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create
	p, err := store.CreateProject(ctx, storage.Project{
		Name:        "Test Project",
		Description: "A test project",
		Environment: "dev",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", p.Name)
	}
	if p.Environment != "dev" {
		t.Errorf("expected environment 'dev', got %q", p.Environment)
	}

	// Get
	got, err := store.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", got.Name)
	}

	// List
	projects, total, err := store.ListProjects(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 total, got %d", total)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}

	// Update
	p.Name = "Updated Project"
	p.Environment = "staging"
	updated, err := store.UpdateProject(ctx, *p)
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if updated.Name != "Updated Project" {
		t.Errorf("expected updated name, got %q", updated.Name)
	}
	if updated.Environment != "staging" {
		t.Errorf("expected environment 'staging', got %q", updated.Environment)
	}

	// Delete
	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	_, err = store.GetProject(ctx, p.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestAPIKeyCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create project first.
	p, err := store.CreateProject(ctx, storage.Project{
		Name: "Key Test Project",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create API key.
	fullKey := GenerateAPIKey()
	key, err := store.CreateAPIKey(ctx, storage.APIKey{
		ProjectID: p.ID,
		Name:      "dev-key",
	}, fullKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key.KeyPrefix != fullKey[:8] {
		t.Errorf("expected prefix %q, got %q", fullKey[:8], key.KeyPrefix)
	}
	if !key.Active {
		t.Error("expected active key")
	}

	// List keys.
	keys, err := store.ListAPIKeys(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}

	// Validate key.
	validated, err := store.ValidateAPIKey(ctx, fullKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if validated.ID != key.ID {
		t.Errorf("expected key ID %q, got %q", key.ID, validated.ID)
	}

	// Wrong key should fail.
	_, err = store.ValidateAPIKey(ctx, "cdla_0000000000000000000000000000")
	if err == nil {
		t.Error("expected error for wrong key")
	}

	// Revoke.
	if err := store.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	// Validation should fail after revoke.
	_, err = store.ValidateAPIKey(ctx, fullKey)
	if err == nil {
		t.Error("expected error for revoked key")
	}
}

func TestCascadeDelete(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p, _ := store.CreateProject(ctx, storage.Project{Name: "Cascade Test"})
	fullKey := GenerateAPIKey()
	store.CreateAPIKey(ctx, storage.APIKey{ProjectID: p.ID, Name: "k1"}, fullKey)

	// Delete project — should cascade to keys.
	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	keys, err := store.ListAPIKeys(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys after delete: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after cascade delete, got %d", len(keys))
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key := GenerateAPIKey()
	if len(key) != 37 { // "cdla_" (5) + 32 hex chars
		t.Errorf("expected 37 char key, got %d: %q", len(key), key)
	}
	if key[:5] != "cdla_" {
		t.Errorf("expected cdla_ prefix, got %q", key[:5])
	}
}
