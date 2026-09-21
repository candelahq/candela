package main

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

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		devMode     bool
		kService    string // set as K_SERVICE env var for the test
		cloudRunURL string
		wantErr     bool
	}{
		{"dev mode on Cloud Run is rejected", true, "candela-server", "https://candela.run.app", true},
		{"dev mode locally is allowed", true, "", "", false},
		{"prod mode on Cloud Run is allowed", false, "candela-server", "https://candela.run.app", false},
		{"prod mode locally is allowed", false, "", "", false},
		{"dev mode on Cloud Run without CLOUD_RUN_URL is rejected", true, "candela-server", "", true},
		{"prod mode on Cloud Run without CLOUD_RUN_URL warns but succeeds", false, "candela-server", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all production indicators so inherited env doesn't leak (#648).
			for _, env := range []string{
				"K_SERVICE", "K_REVISION", "KUBERNETES_SERVICE_HOST",
				"ECS_CONTAINER_METADATA_URI", "AWS_LAMBDA_FUNCTION_NAME",
				"CONTAINER_APP_NAME", "GAE_ENV",
			} {
				t.Setenv(env, "")
			}
			// Set K_SERVICE for this specific test case.
			if tt.kService != "" {
				t.Setenv("K_SERVICE", tt.kService)
			}
			err := validateAuthConfig(tt.devMode, tt.kService, tt.cloudRunURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAuthConfig(devMode=%v, kService=%q, cloudRunURL=%q) error = %v, wantErr %v",
					tt.devMode, tt.kService, tt.cloudRunURL, err, tt.wantErr)
			}
		})
	}
}

// ─── CORS Middleware Tests (#640, #628) ──────────────────────────────────────

func TestCORSMiddleware_AuthErrorHasCORS(t *testing.T) {
	// Dummy auth handler that rejects unauthenticated requests with 401.
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
	})

	allowedOrigins := []string{"https://app.candela.run", "http://localhost:3000"}
	handler := corsMiddleware(authHandler, allowedOrigins)

	req := httptest.NewRequest(http.MethodGet, "/candela.v1.TraceService/ListTraces", nil)
	req.Header.Set("Origin", "https://app.candela.run")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	// #640: Verify CORS headers are present even when auth fails with 401.
	corsOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "https://app.candela.run" {
		t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "https://app.candela.run", corsOrigin)
	}
	if vary := rec.Header().Get("Vary"); vary != "Origin" {
		t.Errorf("expected Vary %q, got %q", "Origin", vary)
	}
}

func TestCORSMiddleware_PreflightOptions(t *testing.T) {
	innerCalled := false
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
	})

	allowedOrigins := []string{"https://app.candela.run"}
	handler := corsMiddleware(innerHandler, allowedOrigins)

	req := httptest.NewRequest(http.MethodOptions, "/candela.v1.TraceService/ListTraces", nil)
	req.Header.Set("Origin", "https://app.candela.run")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 No Content for preflight, got %d", rec.Code)
	}
	if innerCalled {
		t.Errorf("expected inner handler not to be called on preflight OPTIONS")
	}

	corsOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "https://app.candela.run" {
		t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "https://app.candela.run", corsOrigin)
	}
	exposeHeaders := rec.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposeHeaders, "X-Trace-Id") {
		t.Errorf("expected Access-Control-Expose-Headers to contain X-Trace-Id, got %q", exposeHeaders)
	}
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wildcard configured: #628
	handler := corsMiddleware(innerHandler, []string{"*"})

	// When Origin is provided, it should be reflected (to satisfy credentialed CORS).
	reqWithOrigin := httptest.NewRequest(http.MethodGet, "/candela.v1.TraceService/ListTraces", nil)
	reqWithOrigin.Header.Set("Origin", "https://arbitrary-client.com")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, reqWithOrigin)

	if got := rec1.Header().Get("Access-Control-Allow-Origin"); got != "https://arbitrary-client.com" {
		t.Errorf("expected reflected origin %q, got %q", "https://arbitrary-client.com", got)
	}

	// When no Origin is provided, it returns wildcard "*".
	reqNoOrigin := httptest.NewRequest(http.MethodGet, "/candela.v1.TraceService/ListTraces", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, reqNoOrigin)

	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "*", got)
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(innerHandler, []string{"https://app.candela.run"})

	req := httptest.NewRequest(http.MethodGet, "/candela.v1.TraceService/ListTraces", nil)
	req.Header.Set("Origin", "https://unauthorized-attacker.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected empty Access-Control-Allow-Origin for unauthorized origin, got %q", got)
	}
}

// ─── Metrics Handler Tests (#625, #706) ──────────────────────────────────────

type mockMetricsUserStore struct {
	storage.UserStore
	users map[string]*storage.UserRecord
}

func (m *mockMetricsUserStore) GetUser(_ context.Context, id string) (*storage.UserRecord, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return u, nil
}

func TestMetricsHandler_Unauthenticated(t *testing.T) {
	store := &mockMetricsUserStore{users: map[string]*storage.UserRecord{}}
	handler := newMetricsHandler(MetricsDeps{
		UserStore: store,
		DevMode:   false,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated caller, got %d", rec.Code)
	}
}

func TestMetricsHandler_NonAdmin(t *testing.T) {
	store := &mockMetricsUserStore{
		users: map[string]*storage.UserRecord{
			"dev-user": {ID: "dev-user", Email: "dev@candela.run", Role: storage.RoleDeveloper},
		},
	}
	handler := newMetricsHandler(MetricsDeps{
		UserStore: store,
		DevMode:   false,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	ctx := auth.NewContext(req.Context(), &auth.User{ID: "dev-user", Email: "dev@candela.run"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-admin caller, got %d", rec.Code)
	}
}

func TestMetricsHandler_Admin(t *testing.T) {
	store := &mockMetricsUserStore{
		users: map[string]*storage.UserRecord{
			"admin-user": {ID: "admin-user", Email: "admin@candela.run", Role: storage.RoleAdmin},
		},
	}
	handler := newMetricsHandler(MetricsDeps{
		UserStore: store,
		DevMode:   false,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	ctx := auth.NewContext(req.Context(), &auth.User{ID: "admin-user", Email: "admin@candela.run"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin caller, got %d", rec.Code)
	}

	var payload struct {
		Proxy struct {
			DroppedSpans int64   `json:"dropped_spans"`
			SASpendUSD   float64 `json:"sa_spend_usd"`
		} `json:"proxy"`
		Processor struct {
			DroppedSpans int64 `json:"dropped_spans"`
		} `json:"processor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode metrics response: %v", err)
	}
}

func TestMetricsHandler_NilUserStore_DevMode(t *testing.T) {
	handler := newMetricsHandler(MetricsDeps{
		UserStore: nil,
		DevMode:   true,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK when userStore is nil in dev mode, got %d", rec.Code)
	}
}

func TestMetricsHandler_NilUserStore_Prod(t *testing.T) {
	handler := newMetricsHandler(MetricsDeps{
		UserStore: nil,
		DevMode:   false,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when userStore is nil in prod mode, got %d", rec.Code)
	}
}
