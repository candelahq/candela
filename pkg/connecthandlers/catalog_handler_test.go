package connecthandlers

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	types "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// testCatalogEntries is the fixture set for catalog handler tests.
var testCatalogEntries = []catalog.Entry{
	{
		ModelID:          "gemini-2.5-pro",
		Provider:         "google",
		DisplayName:      "Gemini 2.5 Pro",
		InputPerMillion:  1.25,
		OutputPerMillion: 10.00,
		Enabled:          true,
		Category:         "flagship",
		ContextWindow:    1_000_000,
	},
	{
		ModelID:          "claude-sonnet-4",
		Provider:         "anthropic",
		DisplayName:      "Claude Sonnet 4",
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		Enabled:          true,
		Category:         "flagship",
		ContextWindow:    200_000,
	},
	{
		ModelID:          "gpt-4.1",
		Provider:         "openai",
		DisplayName:      "GPT-4.1",
		InputPerMillion:  2.00,
		OutputPerMillion: 8.00,
		Enabled:          false,
		Category:         "flagship",
		ContextWindow:    1_000_000,
	},
}

// mockUserStoreForCatalog is a minimal mock to satisfy scopeUserID checks.
// It returns a user record with the given role when looked up by email.
type mockUserStoreForCatalog struct {
	storage.UserStore // embed to satisfy interface; unused methods panic
	users             map[string]*storage.UserRecord
}

func (m *mockUserStoreForCatalog) GetUserByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, storage.ErrNotFound
}

func newAdminUserStore() *mockUserStoreForCatalog {
	return &mockUserStoreForCatalog{
		users: map[string]*storage.UserRecord{
			"admin@example.com": {
				ID:    "admin@example.com",
				Email: "admin@example.com",
				Role:  storage.RoleAdmin,
			},
		},
	}
}

func newDeveloperUserStore() *mockUserStoreForCatalog {
	return &mockUserStoreForCatalog{
		users: map[string]*storage.UserRecord{
			"dev@example.com": {
				ID:    "dev@example.com",
				Email: "dev@example.com",
				Role:  storage.RoleDeveloper,
			},
		},
	}
}

// adminContext returns a context with an admin user attached.
func adminContext() context.Context {
	return auth.NewContext(context.Background(), &auth.User{
		ID:    "admin-uid",
		Email: "admin@example.com",
	})
}

// developerContext returns a context with a developer (non-admin) user attached.
func developerContext() context.Context {
	return auth.NewContext(context.Background(), &auth.User{
		ID:    "dev-uid",
		Email: "dev@example.com",
	})
}

// TestCatalogHandler_List tests that listing returns enabled entries.
func TestCatalogHandler_List(t *testing.T) {
	store := catalog.NewConfigStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil) // nil users = dev mode = admin-like

	resp, err := handler.ListModelCatalog(context.Background(),
		connect.NewRequest(&v1.ListModelCatalogRequest{}))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}

	// Default (includeDisabled=false): should get 2 enabled entries.
	if len(resp.Msg.Models) != 2 {
		t.Errorf("expected 2 enabled models, got %d", len(resp.Msg.Models))
	}
	if resp.Msg.Source != "config" {
		t.Errorf("expected source 'config', got %q", resp.Msg.Source)
	}
	if resp.Msg.AdminEditable {
		t.Error("expected admin_editable=false for config store")
	}

	// Verify model IDs.
	ids := make(map[string]bool)
	for _, m := range resp.Msg.Models {
		ids[m.ModelId] = true
	}
	if !ids["gemini-2.5-pro"] {
		t.Error("expected gemini-2.5-pro in results")
	}
	if !ids["claude-sonnet-4"] {
		t.Error("expected claude-sonnet-4 in results")
	}
}

// TestCatalogHandler_ListIncludeDisabled_Admin tests that admins can see disabled entries.
func TestCatalogHandler_ListIncludeDisabled_Admin(t *testing.T) {
	store := catalog.NewConfigStore(testCatalogEntries)
	handler := NewCatalogHandler(store, newAdminUserStore())

	resp, err := handler.ListModelCatalog(adminContext(),
		connect.NewRequest(&v1.ListModelCatalogRequest{
			IncludeDisabled: true,
		}))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}

	// Admin with includeDisabled=true: should get all 3 entries.
	if len(resp.Msg.Models) != 3 {
		t.Errorf("expected 3 models (admin + include_disabled), got %d", len(resp.Msg.Models))
	}
}

// TestCatalogHandler_ListDisabled_NonAdmin tests that non-admin callers
// cannot see disabled entries even when requesting include_disabled=true.
func TestCatalogHandler_ListDisabled_NonAdmin(t *testing.T) {
	store := catalog.NewConfigStore(testCatalogEntries)
	handler := NewCatalogHandler(store, newDeveloperUserStore())

	resp, err := handler.ListModelCatalog(developerContext(),
		connect.NewRequest(&v1.ListModelCatalogRequest{
			IncludeDisabled: true, // should be ignored for non-admin
		}))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}

	// Non-admin: includeDisabled is forced to false, should only get 2 enabled entries.
	if len(resp.Msg.Models) != 2 {
		t.Errorf("expected 2 models (non-admin forced enabled-only), got %d", len(resp.Msg.Models))
	}
	for _, m := range resp.Msg.Models {
		if !m.Enabled {
			t.Errorf("non-admin received disabled model: %s", m.ModelId)
		}
	}
}

// TestCatalogHandler_ListAdminEditable_Developer verifies that developers
// do NOT receive admin_editable=true even on a writable store.
func TestCatalogHandler_ListAdminEditable_Developer(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries) // Writable() = true
	handler := NewCatalogHandler(store, newDeveloperUserStore())

	resp, err := handler.ListModelCatalog(developerContext(),
		connect.NewRequest(&v1.ListModelCatalogRequest{}))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}

	if resp.Msg.AdminEditable {
		t.Error("expected admin_editable=false for developer caller on writable store")
	}
}

// TestCatalogHandler_ListAdminEditable_Admin verifies that admins
// receive admin_editable=true on a writable store.
func TestCatalogHandler_ListAdminEditable_Admin(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries) // Writable() = true
	handler := NewCatalogHandler(store, newAdminUserStore())

	resp, err := handler.ListModelCatalog(adminContext(),
		connect.NewRequest(&v1.ListModelCatalogRequest{}))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}

	if !resp.Msg.AdminEditable {
		t.Error("expected admin_editable=true for admin caller on writable store")
	}
}

// TestCatalogHandler_UpdateReadOnly tests that update returns Unimplemented
// when the store is read-only.
func TestCatalogHandler_UpdateReadOnly(t *testing.T) {
	store := catalog.NewConfigStore(testCatalogEntries) // read-only
	handler := NewCatalogHandler(store, nil)

	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:         "test-model",
				Provider:        "test-provider",
				InputPerMillion: 1.00,
			},
		}))

	if err == nil {
		t.Fatal("expected error for read-only store update")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connectErr.Code())
	}
}

// TestCatalogHandler_DeleteReadOnly tests that delete returns Unimplemented
// when the store is read-only.
func TestCatalogHandler_DeleteReadOnly(t *testing.T) {
	store := catalog.NewConfigStore(testCatalogEntries) // read-only
	handler := NewCatalogHandler(store, nil)

	_, err := handler.DeleteModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.DeleteModelCatalogEntryRequest{
			Provider: "google",
			ModelId:  "gemini-2.5-pro",
		}))

	if err == nil {
		t.Fatal("expected error for read-only store delete")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connectErr.Code())
	}
}

// TestCatalogHandler_UpdateNonAdmin tests that non-admin callers are denied
// update access even when the store is writable.
func TestCatalogHandler_UpdateNonAdmin(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, newDeveloperUserStore())

	_, err := handler.UpdateModelCatalogEntry(developerContext(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:         "gemini-2.5-pro",
				Provider:        "google",
				InputPerMillion: 99.99,
			},
		}))

	if err == nil {
		t.Fatal("expected permission denied for non-admin update")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connectErr.Code())
	}
}

// --- mockWritableCatalogStore ---

// mockWritableCatalogStore is a simple in-memory writable catalog store for tests.
type mockWritableCatalogStore struct {
	entries map[string]catalog.Entry // key: provider/modelID
}

func newMockWritableCatalogStore(initial []catalog.Entry) *mockWritableCatalogStore {
	m := &mockWritableCatalogStore{entries: make(map[string]catalog.Entry)}
	for _, e := range initial {
		m.entries[e.Provider+"/"+e.ModelID] = e
	}
	return m
}

func (m *mockWritableCatalogStore) List(_ context.Context, includeDisabled bool) ([]catalog.Entry, error) {
	var out []catalog.Entry
	for _, e := range m.entries {
		if includeDisabled || e.Enabled {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *mockWritableCatalogStore) Get(_ context.Context, provider, modelID string) (*catalog.Entry, error) {
	e, ok := m.entries[provider+"/"+modelID]
	if !ok {
		return nil, catalog.ErrNotFound
	}
	return &e, nil
}

func (m *mockWritableCatalogStore) Update(_ context.Context, entry catalog.Entry) error {
	m.entries[entry.Provider+"/"+entry.ModelID] = entry
	return nil
}

func (m *mockWritableCatalogStore) Delete(_ context.Context, provider, modelID string) error {
	key := provider + "/" + modelID
	if _, ok := m.entries[key]; !ok {
		return catalog.ErrNotFound
	}
	delete(m.entries, key)
	return nil
}

func (m *mockWritableCatalogStore) Source() string { return "mock" }
func (m *mockWritableCatalogStore) Writable() bool { return true }

// TestCatalogHandler_UpdateWithFieldMask verifies that a field mask update
// only modifies the specified fields and preserves all others.
func TestCatalogHandler_UpdateWithFieldMask(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil) // nil users = dev mode = admin

	// Update only "enabled" to false on gemini-2.5-pro.
	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:  "gemini-2.5-pro",
				Provider: "google",
				Enabled:  false,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"enabled"}},
		}))
	if err != nil {
		t.Fatalf("UpdateModelCatalogEntry: %v", err)
	}

	// Re-fetch and verify only enabled changed.
	got, err := store.Get(context.Background(), "google", "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Enabled {
		t.Error("expected enabled=false after masked update")
	}
	// All other fields should be preserved from the original fixture.
	if got.DisplayName != "Gemini 2.5 Pro" {
		t.Errorf("display_name changed: got %q, want %q", got.DisplayName, "Gemini 2.5 Pro")
	}
	if got.InputPerMillion != 1.25 {
		t.Errorf("input_per_million changed: got %v, want 1.25", got.InputPerMillion)
	}
	if got.OutputPerMillion != 10.00 {
		t.Errorf("output_per_million changed: got %v, want 10.00", got.OutputPerMillion)
	}
	if got.Category != "flagship" {
		t.Errorf("category changed: got %q, want %q", got.Category, "flagship")
	}
	if got.ContextWindow != 1_000_000 {
		t.Errorf("context_window changed: got %d, want 1000000", got.ContextWindow)
	}
}

// TestCatalogHandler_UpdateMultipleFieldMask verifies that multiple fields
// in the mask are all applied correctly.
func TestCatalogHandler_UpdateMultipleFieldMask(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil)

	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:         "gemini-2.5-pro",
				Provider:        "google",
				InputPerMillion: 2.50,
				DiscountPercent: 0.10,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{
				"input_per_million", "discount_percent",
			}},
		}))
	if err != nil {
		t.Fatalf("UpdateModelCatalogEntry: %v", err)
	}

	got, err := store.Get(context.Background(), "google", "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.InputPerMillion != 2.50 {
		t.Errorf("input_per_million: got %v, want 2.50", got.InputPerMillion)
	}
	if got.DiscountPercent != 0.10 {
		t.Errorf("discount_percent: got %v, want 0.10", got.DiscountPercent)
	}
	// Unmasked fields preserved.
	if got.OutputPerMillion != 10.00 {
		t.Errorf("output_per_million: got %v, want 10.00 (should be preserved)", got.OutputPerMillion)
	}
	if got.DisplayName != "Gemini 2.5 Pro" {
		t.Errorf("display_name: got %q, want %q (should be preserved)", got.DisplayName, "Gemini 2.5 Pro")
	}
}

// TestCatalogHandler_UpdateEmptyMask_FullReplace verifies that when no field mask
// is provided, all fields are overwritten (full replace semantics).
func TestCatalogHandler_UpdateEmptyMask_FullReplace(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil)

	// Full replace: no update_mask.
	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:         "gemini-2.5-pro",
				Provider:        "google",
				DisplayName:     "Replaced Model",
				InputPerMillion: 99.99,
				Enabled:         false,
			},
		}))
	if err != nil {
		t.Fatalf("UpdateModelCatalogEntry: %v", err)
	}

	got, err := store.Get(context.Background(), "google", "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "Replaced Model" {
		t.Errorf("display_name: got %q, want %q", got.DisplayName, "Replaced Model")
	}
	if got.InputPerMillion != 99.99 {
		t.Errorf("input_per_million: got %v, want 99.99", got.InputPerMillion)
	}
	// Zero values expected since no mask = full replace.
	if got.OutputPerMillion != 0 {
		t.Errorf("output_per_million: got %v, want 0 (full replace)", got.OutputPerMillion)
	}
	if got.ContextWindow != 0 {
		t.Errorf("context_window: got %d, want 0 (full replace)", got.ContextWindow)
	}
}

// TestCatalogHandler_UpdateFieldMask_NotFound verifies that a masked update
// on a non-existent entry returns CodeNotFound.
func TestCatalogHandler_UpdateFieldMask_NotFound(t *testing.T) {
	store := newMockWritableCatalogStore(nil)
	handler := NewCatalogHandler(store, nil)

	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: &types.ModelCatalogEntry{
				ModelId:  "nonexistent",
				Provider: "nowhere",
				Enabled:  true,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"enabled"}},
		}))
	if err == nil {
		t.Fatal("expected error for masked update on non-existent entry")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestCatalogHandler_DeleteNonExistent tests that deleting a model that doesn't
// exist returns CodeNotFound.
func TestCatalogHandler_DeleteNonExistent(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil)

	_, err := handler.DeleteModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.DeleteModelCatalogEntryRequest{
			Provider: "google",
			ModelId:  "nonexistent-model",
		}))
	if err == nil {
		t.Fatal("expected error for deleting non-existent model")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestCatalogHandler_DeleteNonAdmin verifies non-admin callers are denied delete.
func TestCatalogHandler_DeleteNonAdmin(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, newDeveloperUserStore())

	_, err := handler.DeleteModelCatalogEntry(developerContext(),
		connect.NewRequest(&v1.DeleteModelCatalogEntryRequest{
			Provider: "google",
			ModelId:  "gemini-2.5-pro",
		}))
	if err == nil {
		t.Fatal("expected permission denied for non-admin delete")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connectErr.Code())
	}
}

// TestCatalogHandler_ListEmpty tests that listing on an empty store returns
// an empty list, not an error.
func TestCatalogHandler_ListEmpty(t *testing.T) {
	store := newMockWritableCatalogStore(nil) // empty store
	handler := NewCatalogHandler(store, nil)

	resp, err := handler.ListModelCatalog(context.Background(),
		connect.NewRequest(&v1.ListModelCatalogRequest{}))
	if err != nil {
		t.Fatalf("ListModelCatalog on empty store: %v", err)
	}
	if len(resp.Msg.Models) != 0 {
		t.Errorf("expected 0 models from empty store, got %d", len(resp.Msg.Models))
	}
}

// TestCatalogHandler_UpdateNilEntry tests that Update with nil entry returns
// CodeInvalidArgument.
func TestCatalogHandler_UpdateNilEntry(t *testing.T) {
	store := newMockWritableCatalogStore(testCatalogEntries)
	handler := NewCatalogHandler(store, nil)

	_, err := handler.UpdateModelCatalogEntry(context.Background(),
		connect.NewRequest(&v1.UpdateModelCatalogEntryRequest{
			Entry: nil,
		}))
	if err == nil {
		t.Fatal("expected error for nil entry")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}

// TestApplyFieldMask is a unit test for the applyFieldMask helper.
func TestApplyFieldMask(t *testing.T) {
	dst := catalog.Entry{
		ModelID:         "model-1",
		Provider:        "prov",
		DisplayName:     "Original Name",
		InputPerMillion: 1.00,
		Enabled:         true,
		Category:        "flagship",
		ContextWindow:   100_000,
	}
	src := catalog.Entry{
		ModelID:         "model-1",
		Provider:        "prov",
		DisplayName:     "New Name",
		InputPerMillion: 5.00,
		Enabled:         false,
		Category:        "lite",
		ContextWindow:   200_000,
	}

	result := applyFieldMask(dst, src, []string{"enabled", "display_name"})

	if result.Enabled != false {
		t.Error("enabled should be false")
	}
	if result.DisplayName != "New Name" {
		t.Errorf("display_name: got %q, want %q", result.DisplayName, "New Name")
	}
	// Unmasked fields should be from dst.
	if result.InputPerMillion != 1.00 {
		t.Errorf("input_per_million should be 1.00 (from dst), got %v", result.InputPerMillion)
	}
	if result.Category != "flagship" {
		t.Errorf("category should be 'flagship' (from dst), got %q", result.Category)
	}
	if result.ContextWindow != 100_000 {
		t.Errorf("context_window should be 100000 (from dst), got %d", result.ContextWindow)
	}
}
