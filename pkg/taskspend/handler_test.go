package taskspend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
)

// handlerMockStore implements Store for handler tests.
type handlerMockStore struct {
	budget *billing.TaskBudget
	err    error
}

func (m *handlerMockStore) GetTaskBudget(_ context.Context, _ string) (*billing.TaskBudget, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.budget, nil
}

// mockUserLookup implements UserLookup for testing.
type mockUserLookup struct {
	records map[string]*storage.UserRecord
}

func (m *mockUserLookup) GetUser(_ context.Context, id string) (*storage.UserRecord, error) {
	if r, ok := m.records[id]; ok {
		return r, nil
	}
	return nil, storage.ErrNotFound
}

func testBudget() *billing.TaskBudget {
	return &billing.TaskBudget{
		TaskID:    "task-123",
		UserID:    "owner@example.com",
		LimitUSD:  10.0,
		SpentUSD:  3.0,
		CreatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func newTestCache(budget *billing.TaskBudget) *Cache {
	store := &handlerMockStore{budget: budget}
	return New(store, DefaultTTL)
}

func doRequest(handler http.HandlerFunc, taskID string, user *auth.User) *httptest.ResponseRecorder {
	return doRequestMethod(handler, http.MethodGet, taskID, user)
}

func doRequestMethod(handler http.HandlerFunc, method, taskID string, user *auth.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/task-spend/"+taskID, nil)
	if user != nil {
		req = req.WithContext(auth.NewContext(req.Context(), user))
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ── Test: non-GET → 405 ──

func TestHandler_NonGET_Returns405(t *testing.T) {
	h := Handler(newTestCache(testBudget()), nil)
	owner := &auth.User{ID: "owner@example.com", Email: "owner@example.com"}
	rr := doRequestMethod(h, http.MethodPost, "task-123", owner)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// ── Test: empty task ID → 400 ──

func TestHandler_EmptyTaskID_Returns400(t *testing.T) {
	h := Handler(newTestCache(testBudget()), nil)
	owner := &auth.User{ID: "owner@example.com", Email: "owner@example.com"}
	rr := doRequest(h, "", owner)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ── Test: extractTaskID ──

func TestExtractTaskID(t *testing.T) {
	cases := map[string]string{
		"/api/v1/task-spend/task-123":  "task-123",
		"/api/v1/task-spend/task-123/": "task-123",
		"/api/v1/task-spend/":          "",
		"/api/v1/other/task-123":       "",
	}
	for path, want := range cases {
		if got := extractTaskID(path); got != want {
			t.Errorf("extractTaskID(%q) = %q, want %q", path, got, want)
		}
	}
}

// ── Test: no auth → 401 ──

func TestHandler_NoAuth_Returns401(t *testing.T) {
	cache := newTestCache(testBudget())
	h := Handler(cache, nil)

	rr := doRequest(h, "task-123", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// ── Test: owner sees own task → 200 ──

func TestHandler_Owner_Returns200(t *testing.T) {
	cache := newTestCache(testBudget())
	h := Handler(cache, nil)

	owner := &auth.User{ID: "owner@example.com", Email: "owner@example.com"}
	rr := doRequest(h, "task-123", owner)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var snap SpendSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.TaskID != "task-123" {
		t.Errorf("task_id = %q, want 'task-123'", snap.TaskID)
	}
}

// ── Test: non-owner, non-admin → 404 ──

func TestHandler_NonOwner_Returns404(t *testing.T) {
	cache := newTestCache(testBudget())
	users := &mockUserLookup{
		records: map[string]*storage.UserRecord{
			"other@example.com": {Role: "member"},
		},
	}
	h := Handler(cache, users)

	other := &auth.User{ID: "other@example.com", Email: "other@example.com"}
	rr := doRequest(h, "task-123", other)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

// ── Test: admin sees any task → 200 ──

func TestHandler_Admin_Returns200(t *testing.T) {
	cache := newTestCache(testBudget())
	users := &mockUserLookup{
		records: map[string]*storage.UserRecord{
			"admin@example.com": {Role: storage.RoleAdmin},
		},
	}
	h := Handler(cache, users)

	admin := &auth.User{ID: "admin@example.com", Email: "admin@example.com"}
	rr := doRequest(h, "task-123", admin)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

// ── Test: SA sees any task → 200 ──

func TestHandler_SA_Returns200(t *testing.T) {
	cache := newTestCache(testBudget())
	h := Handler(cache, nil)

	sa := &auth.User{
		ID:    "bot@project.iam.gserviceaccount.com",
		Email: "bot@project.iam.gserviceaccount.com",
	}
	rr := doRequest(h, "task-123", sa)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

// ── Test: missing task → 404 ──

func TestHandler_MissingTask_Returns404(t *testing.T) {
	store := &handlerMockStore{err: storage.ErrNotFound}
	cache := New(store, DefaultTTL)
	h := Handler(cache, nil)

	user := &auth.User{ID: "anyone@example.com", Email: "anyone@example.com"}
	rr := doRequest(h, "no-such-task", user)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// ── Test: OwnerID not leaked in JSON response ──

func TestHandler_OwnerIDNotInJSON(t *testing.T) {
	cache := newTestCache(testBudget())
	h := Handler(cache, nil)

	owner := &auth.User{ID: "owner@example.com", Email: "owner@example.com"}
	rr := doRequest(h, "task-123", owner)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	// Verify raw JSON doesn't contain owner_id.
	body := rr.Body.String()
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, exists := raw["owner_id"]; exists {
		t.Error("owner_id should not appear in JSON response")
	}
}
