package taskspend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
)

func TestHandler_HappyPath(t *testing.T) {
	store := &mockStore{
		budget: &billing.TaskBudget{
			TaskID:   "job-http-1",
			LimitUSD: 10.0,
			SpentUSD: 2.5,
		},
	}
	cache := New(store, 5*time.Second)
	handler := Handler(cache)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-spend/job-http-1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var snap SpendSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.TaskID != "job-http-1" {
		t.Errorf("TaskID = %q, want %q", snap.TaskID, "job-http-1")
	}
	if snap.RemainingUSD != 7.5 {
		t.Errorf("RemainingUSD = %v, want 7.5", snap.RemainingUSD)
	}
}

func TestHandler_NotFound(t *testing.T) {
	store := &mockStore{err: storage.ErrNotFound}
	cache := New(store, 5*time.Second)
	handler := Handler(cache)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-spend/missing", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_MissingTaskID(t *testing.T) {
	cache := New(&mockStore{}, 5*time.Second)
	handler := Handler(cache)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-spend/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	cache := New(&mockStore{}, 5*time.Second)
	handler := Handler(cache)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-spend/job-1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/task-spend/job-123", "job-123"},
		{"/api/v1/task-spend/job-123/", "job-123"},
		{"/api/v1/task-spend/", ""},
		{"/api/v1/task-spend", ""},
		{"/other/path", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := extractTaskID(tt.path); got != tt.want {
				t.Errorf("extractTaskID(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
