package modellimits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// mockStore implements Store for testing.
type mockStore struct {
	limits     []*storage.ModelLimitRecord
	setErr     error
	listErr    error
	deleteErr  error
	setCalled  int
	delCalled  int
	lastLimit  *storage.ModelLimitRecord
	lastDelUID string
	lastDelPfx string
}

func (m *mockStore) SetModelLimit(_ context.Context, limit *storage.ModelLimitRecord) error {
	m.setCalled++
	m.lastLimit = limit
	if m.setErr != nil {
		return m.setErr
	}
	m.limits = append(m.limits, limit)
	return nil
}

func (m *mockStore) GetModelLimits(_ context.Context, _ string) ([]*storage.ModelLimitRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.limits, nil
}

func (m *mockStore) DeleteModelLimit(_ context.Context, userID, modelPrefix string) error {
	m.delCalled++
	m.lastDelUID = userID
	m.lastDelPfx = modelPrefix
	return m.deleteErr
}

// mockUserLookup implements UserLookup for testing.
type mockUserLookup struct {
	role string
}

func (m *mockUserLookup) GetUser(_ context.Context, _ string) (*storage.UserRecord, error) {
	return &storage.UserRecord{Role: m.role}, nil
}

func withAuth(h http.Handler, userID, email, role string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: email})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestHandler_Unauthenticated(t *testing.T) {
	h := Handler(&mockStore{}, &mockUserLookup{role: "admin"})
	req := httptest.NewRequest("GET", "/api/v1/users/alice/model-limits", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandler_NonAdmin(t *testing.T) {
	h := Handler(&mockStore{}, &mockUserLookup{role: "developer"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "dev-user", "dev@test.com", "developer"))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users/alice/model-limits", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandler_SetModelLimit(t *testing.T) {
	store := &mockStore{}
	h := Handler(store, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	body := `{"max_daily_usd": 25.00}`
	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/users/alice/model-limits/claude-opus-4",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if store.setCalled != 1 {
		t.Errorf("SetModelLimit called %d times, want 1", store.setCalled)
	}
	if store.lastLimit.UserID != "alice" {
		t.Errorf("UserID = %q, want 'alice'", store.lastLimit.UserID)
	}
	if store.lastLimit.ModelPrefix != "claude-opus-4" {
		t.Errorf("ModelPrefix = %q, want 'claude-opus-4'", store.lastLimit.ModelPrefix)
	}
	if store.lastLimit.MaxDailyUSD != 25.00 {
		t.Errorf("MaxDailyUSD = %f, want 25.00", store.lastLimit.MaxDailyUSD)
	}
}

func TestHandler_SetModelLimit_InvalidAmount(t *testing.T) {
	store := &mockStore{}
	h := Handler(store, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	body := `{"max_daily_usd": -5.00}`
	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/users/alice/model-limits/claude-opus-4",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if store.setCalled != 0 {
		t.Errorf("SetModelLimit should not be called for invalid amount")
	}
}

func TestHandler_ListModelLimits(t *testing.T) {
	store := &mockStore{
		limits: []*storage.ModelLimitRecord{
			{UserID: "alice", ModelPrefix: "claude-opus-4", MaxDailyUSD: 50.00},
			{UserID: "alice", ModelPrefix: "gpt-4o", MaxDailyUSD: 25.00},
		},
	}
	h := Handler(store, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	req, _ := http.NewRequest("GET",
		srv.URL+"/api/v1/users/alice/model-limits", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		UserID string                      `json:"user_id"`
		Limits []*storage.ModelLimitRecord `json:"limits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Limits) != 2 {
		t.Errorf("got %d limits, want 2", len(result.Limits))
	}
}

func TestHandler_DeleteModelLimit(t *testing.T) {
	store := &mockStore{}
	h := Handler(store, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	req, _ := http.NewRequest("DELETE",
		srv.URL+"/api/v1/users/alice/model-limits/claude-opus-4", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if store.delCalled != 1 {
		t.Errorf("DeleteModelLimit called %d times, want 1", store.delCalled)
	}
	if store.lastDelUID != "alice" {
		t.Errorf("userID = %q, want 'alice'", store.lastDelUID)
	}
	if store.lastDelPfx != "claude-opus-4" {
		t.Errorf("modelPrefix = %q, want 'claude-opus-4'", store.lastDelPfx)
	}
}

func TestHandler_SetMissingModelPrefix(t *testing.T) {
	h := Handler(&mockStore{}, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	body := `{"max_daily_usd": 25.00}`
	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/users/alice/model-limits",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := Handler(&mockStore{}, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	req, _ := http.NewRequest("PATCH",
		srv.URL+"/api/v1/users/alice/model-limits/claude-opus-4", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		path       string
		wantUser   string
		wantPrefix string
	}{
		{"/api/v1/users/alice/model-limits", "alice", ""},
		{"/api/v1/users/alice/model-limits/claude-opus-4", "alice", "claude-opus-4"},
		{"/api/v1/users/alice/model-limits/gpt-4o/", "alice", "gpt-4o"},
		{"/api/v1/users/bob@example.com/model-limits/gemini-2.5-flash", "bob@example.com", "gemini-2.5-flash"},
		{"/api/v1/other/path", "", ""},
		{"", "", ""},
		// CR #2: Exact segment matching — reject "model-limits-extra".
		{"/api/v1/users/alice/model-limits-extra", "", ""},
		{"/api/v1/users/alice/model-limitsx/foo", "", ""},
		// Only users prefix, no model-limits segment.
		{"/api/v1/users/alice/other-segment", "", ""},
	}

	for _, tt := range tests {
		user, prefix := parsePath(tt.path)
		if user != tt.wantUser || prefix != tt.wantPrefix {
			t.Errorf("parsePath(%q) = (%q, %q), want (%q, %q)",
				tt.path, user, prefix, tt.wantUser, tt.wantPrefix)
		}
	}
}

func TestHandler_GET_RejectsModelPrefix(t *testing.T) {
	h := Handler(&mockStore{limits: []*storage.ModelLimitRecord{
		{UserID: "alice", ModelPrefix: "gpt-4o", MaxDailyUSD: 10},
	}}, &mockUserLookup{role: "admin"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", h)
	srv := httptest.NewServer(withAuth(mux, "admin-user", "admin@test.com", "admin"))
	defer srv.Close()

	// GET with model prefix should be rejected.
	req, _ := http.NewRequest("GET",
		srv.URL+"/api/v1/users/alice/model-limits/gpt-4o", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET with model prefix: status = %d, want 400", resp.StatusCode)
	}
}
