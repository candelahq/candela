package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// ── Mocks ────────────────────────────────────────────────────────────────

type mockBudgetStore struct {
	budget  *storage.BudgetRecord
	budgErr error
	history []storage.DailySpendRecord
	histErr error
}

func (m *mockBudgetStore) GetBudget(_ context.Context, _ string) (*storage.BudgetRecord, error) {
	return m.budget, m.budgErr
}

func (m *mockBudgetStore) GetSpendHistory(_ context.Context, _ string, _ int) ([]storage.DailySpendRecord, error) {
	return m.history, m.histErr
}

type mockUserLookup struct {
	user *storage.UserRecord
	err  error
}

func (m *mockUserLookup) GetUser(_ context.Context, _ string) (*storage.UserRecord, error) {
	return m.user, m.err
}

// helper to create a request with auth context.
func reqWithAuth(method, path string, id auth.Identity) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	return r.WithContext(auth.NewContext(r.Context(), &id))
}

// ── Tests ────────────────────────────────────────────────────────────────

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := Handler(&mockBudgetStore{}, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodPost, "/api/v1/users/alice/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandler_MissingUserID(t *testing.T) {
	h := Handler(&mockBudgetStore{}, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_Unauthenticated(t *testing.T) {
	h := Handler(&mockBudgetStore{}, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice/budget-forecast", nil)

	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_NonAdminAccessingOtherUser(t *testing.T) {
	users := &mockUserLookup{user: &storage.UserRecord{Role: "user"}}
	h := Handler(&mockBudgetStore{}, users)
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/bob/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (non-admin sees 404)", w.Code, http.StatusNotFound)
	}
}

func TestHandler_AdminCanAccessOtherUser(t *testing.T) {
	store := &mockBudgetStore{
		budget: &storage.BudgetRecord{LimitUSD: 50, SpentUSD: 10},
	}
	users := &mockUserLookup{user: &storage.UserRecord{Role: "admin"}}
	h := Handler(store, users)
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/bob/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.SpendHistory == nil {
		t.Error("SpendHistory is nil, want empty slice")
	}
}

func TestHandler_SelfService(t *testing.T) {
	store := &mockBudgetStore{
		budget: &storage.BudgetRecord{LimitUSD: 25, SpentUSD: 12.5},
		history: []storage.DailySpendRecord{
			{Date: "2026-08-22", SpendUSD: 20.0, TokenCount: 100},
		},
	}
	users := &mockUserLookup{}
	h := Handler(store, users)
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/alice/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", w.Header().Get("Content-Type"))
	}

	var result Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.BurnRatePerHour <= 0 {
		t.Error("BurnRatePerHour should be > 0")
	}
	if len(result.SpendHistory) != 1 {
		t.Errorf("SpendHistory len = %d, want 1", len(result.SpendHistory))
	}
}

func TestHandler_NoBudget(t *testing.T) {
	store := &mockBudgetStore{budget: nil}
	h := Handler(store, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/alice/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.DaysUntilExhaustion != -1 {
		t.Errorf("DaysUntilExhaustion = %d, want -1 (no budget)", result.DaysUntilExhaustion)
	}
}

func TestHandler_BudgetStoreError(t *testing.T) {
	store := &mockBudgetStore{budgErr: errors.New("firestore unavailable")}
	h := Handler(store, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/alice/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandler_HistoryErrorIsNonFatal(t *testing.T) {
	store := &mockBudgetStore{
		budget:  &storage.BudgetRecord{LimitUSD: 25, SpentUSD: 5},
		histErr: errors.New("firestore timeout"),
	}
	h := Handler(store, &mockUserLookup{})
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/alice/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	// Should still succeed — history error is non-fatal.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (history error is non-fatal)", w.Code, http.StatusOK)
	}

	var result Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No history → no avg, but burn rate should still work.
	if result.BurnRatePerHour <= 0 {
		t.Error("BurnRatePerHour should be > 0 even without history")
	}
	if result.DaysUntilExhaustion != -1 {
		t.Errorf("DaysUntilExhaustion = %d, want -1 (no history)", result.DaysUntilExhaustion)
	}
}

func TestHandler_OwnerCanAccessOtherUser(t *testing.T) {
	store := &mockBudgetStore{
		budget: &storage.BudgetRecord{LimitUSD: 100, SpentUSD: 30},
	}
	users := &mockUserLookup{user: &storage.UserRecord{Role: "owner"}}
	h := Handler(store, users)
	w := httptest.NewRecorder()
	r := reqWithAuth(http.MethodGet, "/api/v1/users/bob/budget-forecast", auth.Identity{ID: "alice"})

	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (owner should have admin access)", w.Code, http.StatusOK)
	}
}

// ── parseUserID ──────────────────────────────────────────────────────────

func TestParseUserID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/users/alice/budget-forecast", "alice"},
		{"/api/v1/users/bob-123/budget-forecast", "bob-123"},
		{"/api/v1/users//budget-forecast", ""},            // empty userID
		{"/api/v1/users/alice/model-limits", ""},          // wrong suffix
		{"/api/v1/users/alice", ""},                       // too few segments
		{"/api/v1/users/alice/budget-forecast/extra", ""}, // too many segments
		{"/wrong/v1/users/alice/budget-forecast", ""},     // wrong prefix
	}
	for _, tt := range tests {
		got := parseUserID(tt.path)
		if got != tt.want {
			t.Errorf("parseUserID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
