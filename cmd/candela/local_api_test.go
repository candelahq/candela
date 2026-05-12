package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/runtime"
)

// apiMockRuntime is a test double for the Runtime interface.
type apiMockRuntime struct {
	healthy    bool
	models     []runtime.Model
	listErr    error // if set, ListModels returns this error
	pullCalled []string
	mu         sync.Mutex
}

func (m *apiMockRuntime) Name() string     { return "test" }
func (m *apiMockRuntime) Endpoint() string { return "http://127.0.0.1:9999/v1" }
func (m *apiMockRuntime) Start(_ context.Context) error {
	return nil
}
func (m *apiMockRuntime) Stop(_ context.Context) error { return nil }
func (m *apiMockRuntime) Health(_ context.Context) (*runtime.Health, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := runtime.StatusStopped
	if m.healthy {
		status = runtime.StatusRunning
	}
	return &runtime.Health{
		Status:    status,
		Endpoint:  m.Endpoint(),
		Models:    m.models,
		CheckedAt: time.Now(),
	}, nil
}
func (m *apiMockRuntime) ListModels(_ context.Context) ([]runtime.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.models, nil
}
func (m *apiMockRuntime) PullModel(_ context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	m.mu.Lock()
	m.pullCalled = append(m.pullCalled, modelID)
	m.mu.Unlock()
	if progress != nil {
		progress <- runtime.PullProgress{Status: "done", Percent: 100}
	}
	return nil
}

func (m *apiMockRuntime) LoadModel(_ context.Context, _ string) error   { return nil }
func (m *apiMockRuntime) UnloadModel(_ context.Context, _ string) error { return nil }
func (m *apiMockRuntime) DeleteModel(_ context.Context, _ string) error { return nil }

func setupAPI(t *testing.T) (*http.ServeMux, *apiMockRuntime) {
	t.Helper()
	mock := &apiMockRuntime{
		healthy: true,
		models: []runtime.Model{
			{ID: "llama3.2:8b", SizeBytes: 4_700_000_000, Loaded: true},
			{ID: "codellama:13b", SizeBytes: 7_300_000_000, Loaded: true},
		},
	}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 10 * time.Second,
	})
	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(ctx) })

	mux := http.NewServeMux()
	registerLocalAPI(mux, mgr)

	// Poll for the health loop's initial check to complete.
	for i := 0; i < 10; i++ {
		if mgr.Health().Status == runtime.StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	return mux, mock
}

func TestAPIHealth(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/_local/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp runtime.Health
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Status != runtime.StatusRunning {
		t.Errorf("status = %q, want %q", resp.Status, runtime.StatusRunning)
	}
}

func TestAPIListModels(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/_local/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Models []runtime.Model `json:"models"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(resp.Models))
	}
	if resp.Models[0].ID != "llama3.2:8b" {
		t.Errorf("models[0].ID = %q, want %q", resp.Models[0].ID, "llama3.2:8b")
	}
}

func TestAPIPullModel(t *testing.T) {
	mux, mock := setupAPI(t)

	body := `{"model": "mistral:7b"}`
	req := httptest.NewRequest(http.MethodPost, "/_local/models/pull", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["status"] != "pulling" {
		t.Errorf("status = %q, want %q", resp["status"], "pulling")
	}
	if resp["model"] != "mistral:7b" {
		t.Errorf("model = %q, want %q", resp["model"], "mistral:7b")
	}

	// Poll for background pull goroutine to complete.
	for i := 0; i < 10; i++ {
		mock.mu.Lock()
		n := len(mock.pullCalled)
		mock.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mock.mu.Lock()
	pulled := append([]string{}, mock.pullCalled...)
	mock.mu.Unlock()
	if len(pulled) != 1 || pulled[0] != "mistral:7b" {
		t.Errorf("pullCalled = %v, want [mistral:7b]", pulled)
	}
}

func TestAPIPullModel_EmptyModel(t *testing.T) {
	mux, _ := setupAPI(t)

	body := `{"model": ""}`
	req := httptest.NewRequest(http.MethodPost, "/_local/models/pull", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAPIPullModel_BadJSON(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/_local/models/pull", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAPIBackends(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/_local/backends", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Backends []string `json:"backends"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Backends) < 3 {
		t.Errorf("expected at least 3 backends, got %d: %v", len(resp.Backends), resp.Backends)
	}
}

func TestAPIHealth_WrongMethod(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/_local/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /_local/health status = %d, want 405", w.Code)
	}
}

func TestAPIPullModel_WrongMethod(t *testing.T) {
	mux, _ := setupAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/_local/models/pull", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /_local/models/pull status = %d, want 405", w.Code)
	}
}

func TestAPIListModels_RuntimeDown(t *testing.T) {
	mux, mock := setupAPI(t)

	// Simulate runtime being unreachable.
	mock.mu.Lock()
	mock.listErr = fmt.Errorf("connection refused")
	mock.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/_local/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}
