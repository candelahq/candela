package connecthandlers

import (
	"context"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
)

// mockProjectStore is an in-memory ProjectStore for handler tests.
type mockProjectStore struct {
	projects map[string]*storage.Project
	keys     map[string]*storage.APIKey
}

func newMockProjectStore() *mockProjectStore {
	return &mockProjectStore{
		projects: make(map[string]*storage.Project),
		keys:     make(map[string]*storage.APIKey),
	}
}

func (m *mockProjectStore) CreateProject(_ context.Context, p storage.Project) (*storage.Project, error) {
	p.ID = fmt.Sprintf("proj_%d", len(m.projects)+1)
	m.projects[p.ID] = &p
	return &p, nil
}

func (m *mockProjectStore) GetProject(_ context.Context, id string) (*storage.Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func (m *mockProjectStore) ListProjects(_ context.Context, limit, offset int) ([]storage.Project, int, error) {
	var all []storage.Project
	for _, p := range m.projects {
		all = append(all, *p)
	}
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *mockProjectStore) UpdateProject(_ context.Context, p storage.Project) (*storage.Project, error) {
	if _, ok := m.projects[p.ID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	m.projects[p.ID] = &p
	return &p, nil
}

func (m *mockProjectStore) DeleteProject(_ context.Context, id string) error {
	if _, ok := m.projects[id]; !ok {
		return fmt.Errorf("not found")
	}
	delete(m.projects, id)
	return nil
}

func (m *mockProjectStore) CreateAPIKey(_ context.Context, key storage.APIKey, fullKey string) (*storage.APIKey, error) {
	key.ID = fmt.Sprintf("key_%d", len(m.keys)+1)
	key.KeyPrefix = fullKey[:8]
	key.Active = true
	m.keys[key.ID] = &key
	return &key, nil
}

func (m *mockProjectStore) ListAPIKeys(_ context.Context, projectID string) ([]storage.APIKey, error) {
	var keys []storage.APIKey
	for _, k := range m.keys {
		if k.ProjectID == projectID {
			keys = append(keys, *k)
		}
	}
	return keys, nil
}

func (m *mockProjectStore) RevokeAPIKey(_ context.Context, id string) error {
	k, ok := m.keys[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	k.Active = false
	return nil
}

func (m *mockProjectStore) ValidateAPIKey(_ context.Context, rawKey string) (*storage.APIKey, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// --- Handler Tests ---

func TestProjectHandler_CreateProject(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	resp, err := handler.CreateProject(context.Background(),
		connect.NewRequest(&v1.CreateProjectRequest{
			Name:        "Test Project",
			Description: "A test",
		}))

	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if resp.Msg.Project == nil {
		t.Fatal("expected project in response")
	}
	if resp.Msg.Project.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", resp.Msg.Project.Name)
	}
	if resp.Msg.Project.Id == "" {
		t.Error("expected non-empty project ID")
	}
}

func TestProjectHandler_GetProject_NotFound(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	_, err := handler.GetProject(context.Background(),
		connect.NewRequest(&v1.GetProjectRequest{Id: "nonexistent"}))

	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestProjectHandler_ListProjects_Pagination(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	// Create 3 projects.
	for i := 0; i < 3; i++ {
		store.CreateProject(context.Background(), storage.Project{
			Name: fmt.Sprintf("Project %d", i),
		})
	}

	// List with default pagination.
	resp, err := handler.ListProjects(context.Background(),
		connect.NewRequest(&v1.ListProjectsRequest{}))
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(resp.Msg.Projects) != 3 {
		t.Errorf("expected 3 projects, got %d", len(resp.Msg.Projects))
	}
	if resp.Msg.Pagination.TotalCount != 3 {
		t.Errorf("expected total 3, got %d", resp.Msg.Pagination.TotalCount)
	}

	// List with page size.
	resp, err = handler.ListProjects(context.Background(),
		connect.NewRequest(&v1.ListProjectsRequest{
			Pagination: &typespb.PaginationRequest{PageSize: 2},
		}))
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(resp.Msg.Projects) != 2 {
		t.Errorf("expected 2 projects with page_size=2, got %d", len(resp.Msg.Projects))
	}
}

func TestProjectHandler_DeleteProject_NotFound(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	_, err := handler.DeleteProject(context.Background(),
		connect.NewRequest(&v1.DeleteProjectRequest{Id: "nonexistent"}))

	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestProjectHandler_CreateAPIKey(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	// Create a project first.
	p, _ := store.CreateProject(context.Background(), storage.Project{Name: "Key Project"})

	resp, err := handler.CreateAPIKey(context.Background(),
		connect.NewRequest(&v1.CreateAPIKeyRequest{
			ProjectId: p.ID,
			Name:      "dev-key",
		}))
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if resp.Msg.FullKey == "" {
		t.Error("expected full_key at creation time")
	}
	if resp.Msg.ApiKey == nil {
		t.Fatal("expected api_key in response")
	}
	if resp.Msg.ApiKey.Name != "dev-key" {
		t.Errorf("expected name 'dev-key', got %q", resp.Msg.ApiKey.Name)
	}
	if !resp.Msg.ApiKey.Active {
		t.Error("expected active key")
	}
}

func TestProjectHandler_RevokeAPIKey_NotFound(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	_, err := handler.RevokeAPIKey(context.Background(),
		connect.NewRequest(&v1.RevokeAPIKeyRequest{Id: "nonexistent"}))

	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestProjectHandler_FullLifecycle(t *testing.T) {
	store := newMockProjectStore()
	handler := NewProjectHandler(store)

	// Create project.
	createResp, err := handler.CreateProject(context.Background(),
		connect.NewRequest(&v1.CreateProjectRequest{
			Name:        "Lifecycle Test",
			Description: "full lifecycle",
		}))
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := createResp.Msg.Project.Id

	// Get project.
	getResp, err := handler.GetProject(context.Background(),
		connect.NewRequest(&v1.GetProjectRequest{Id: projectID}))
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if getResp.Msg.Project.Name != "Lifecycle Test" {
		t.Errorf("expected 'Lifecycle Test', got %q", getResp.Msg.Project.Name)
	}

	// Create API key.
	keyResp, err := handler.CreateAPIKey(context.Background(),
		connect.NewRequest(&v1.CreateAPIKeyRequest{
			ProjectId: projectID,
			Name:      "test-key",
		}))
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	keyID := keyResp.Msg.ApiKey.Id

	// List keys.
	listKeysResp, err := handler.ListAPIKeys(context.Background(),
		connect.NewRequest(&v1.ListAPIKeysRequest{ProjectId: projectID}))
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(listKeysResp.Msg.ApiKeys) != 1 {
		t.Errorf("expected 1 key, got %d", len(listKeysResp.Msg.ApiKeys))
	}

	// Revoke key.
	_, err = handler.RevokeAPIKey(context.Background(),
		connect.NewRequest(&v1.RevokeAPIKeyRequest{Id: keyID}))
	if err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	// Delete project.
	_, err = handler.DeleteProject(context.Background(),
		connect.NewRequest(&v1.DeleteProjectRequest{Id: projectID}))
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// Confirm deleted.
	_, err = handler.GetProject(context.Background(),
		connect.NewRequest(&v1.GetProjectRequest{Id: projectID}))
	if err == nil {
		t.Error("expected error after delete")
	}
}
