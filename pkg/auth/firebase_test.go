package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFirebaseAuthMiddleware_DevMode(t *testing.T) {
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true)
	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["id"] != "dev-admin" {
		t.Errorf("id = %q, want %q", body["id"], "dev-admin")
	}
	if body["email"] != "admin@localhost" {
		t.Errorf("email = %q, want %q", body["email"], "admin@localhost")
	}
}

func TestFirebaseAuthMiddleware_DevMode_AllPaths(t *testing.T) {
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true)

	paths := []string{
		"/api/data",
		"/proxy/openai/v1/chat/completions",
		"/candela.v1.UserService/GetCurrentUser",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}

			var body map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&body)
			if body["id"] != "dev-admin" {
				t.Errorf("expected dev-admin user for path %s", path)
			}
		})
	}
}

func TestFirebaseAuthMiddleware_HealthCheckBypass(t *testing.T) {
	// Even in prod mode (devMode=false), /healthz should pass without auth.
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := FirebaseAuthMiddleware(healthHandler, nil, "", nil, false)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "ok" {
		t.Errorf("healthz body = %q, want %q", string(body), "ok")
	}
}

func TestFirebaseAuthMiddleware_MissingHeader(t *testing.T) {
	// No Firebase client, no Cloud Run audience — just test missing auth.
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false)

	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}

	var errResp map[string]string
	body, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error: %v — body: %s", err, body)
	}
	if errResp["error"] != "missing authentication" {
		t.Errorf("error = %q, want %q", errResp["error"], "missing authentication")
	}
}

func TestFirebaseAuthMiddleware_InvalidToken_NoValidators(t *testing.T) {
	// With no Firebase client and no Cloud Run audience, any token is invalid.
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false)

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Authorization", "Bearer some-random-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}

	var errResp map[string]string
	body, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error: %v — body: %s", err, body)
	}
	if errResp["error"] != "invalid authentication token" {
		t.Errorf("error = %q, want %q", errResp["error"], "invalid authentication token")
	}
}

func TestFirebaseAuthMiddleware_MalformedAuthHeader(t *testing.T) {
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false)

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "Token some-token"},
		{"empty bearer", "Bearer "},
		{"just bearer", "Bearer"},
		{"basic auth", "Basic dXNlcjpwYXNz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/data", nil)
			req.Header.Set("Authorization", tt.header)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 for header %q", rr.Code, tt.header)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid", "Bearer my-token-123", "my-token-123"},
		{"case insensitive", "bearer my-token", "my-token"},
		{"BEARER caps", "BEARER my-token", "my-token"},
		{"empty", "", ""},
		{"no bearer", "Token abc", ""},
		{"just bearer", "Bearer", ""},
		{"bearer with spaces in token", "Bearer eyJ0eXAi.payload.sig", "eyJ0eXAi.payload.sig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(req)
			if got != tt.want {
				t.Errorf("extractBearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirebaseAuthMiddleware_DevMode_IgnoresAuthHeader(t *testing.T) {
	// In dev mode, even if an auth header is present, the synthetic user is used.
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true)

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Authorization", "Bearer some-real-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["id"] != "dev-admin" {
		t.Errorf("id = %q, want %q — dev mode should ignore real tokens", body["id"], "dev-admin")
	}
}

func TestFirebaseAuthMiddleware_HealthzDoesNotInjectUser(t *testing.T) {
	// /healthz should pass through without any user in context.
	handler := FirebaseAuthMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := FromContext(r.Context())
			if user != nil {
				t.Errorf("expected nil user from healthz, got %+v", user)
			}
			w.WriteHeader(http.StatusOK)
		}),
		nil, "", nil, false,
	)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
