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
