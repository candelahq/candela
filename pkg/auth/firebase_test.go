package auth

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestVerifyRegistered_RegisteredUserAllowed(t *testing.T) {
	// Mock: user exists in store.
	authorizer := UserAuthorizer(func(_ context.Context, email string) error {
		if email == "larry.david@example-corp.com" {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrNotRegistered, email)
	})

	user := &User{ID: "uid-larry", Email: "larry.david@example-corp.com"}
	rr := httptest.NewRecorder()

	allowed := verifyRegistered(context.Background(), rr, user, authorizer)
	if !allowed {
		t.Fatal("expected registered user to be allowed")
	}
	// No error response should have been written.
	if rr.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rr.Body.String())
	}
}

func TestVerifyRegistered_UnregisteredUserBlocked(t *testing.T) {
	// Mock: user does NOT exist in store → ErrNotRegistered.
	authorizer := UserAuthorizer(func(_ context.Context, email string) error {
		return fmt.Errorf("%w: %s", ErrNotRegistered, email)
	})

	user := &User{ID: "uid-rando", Email: "mocha.joe@spite-store.com"}
	rr := httptest.NewRecorder()

	allowed := verifyRegistered(context.Background(), rr, user, authorizer)
	if allowed {
		t.Fatal("expected unregistered user to be blocked")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}

	var errResp map[string]string
	body, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error: %v — body: %s", err, body)
	}
	if errResp["error"] != "user not registered — contact your admin" {
		t.Errorf("error = %q, want user not registered message", errResp["error"])
	}
}

func TestVerifyRegistered_TransientErrorReturns500(t *testing.T) {
	// Mock: Firestore is down — transient error, NOT ErrNotRegistered.
	// Legitimate users should NOT get 403 when the store is unreachable.
	authorizer := UserAuthorizer(func(_ context.Context, _ string) error {
		return fmt.Errorf("firestore: connection refused")
	})

	user := &User{ID: "uid-jeff", Email: "jeff.greene@example-corp.com"}
	rr := httptest.NewRecorder()

	allowed := verifyRegistered(context.Background(), rr, user, authorizer)
	if allowed {
		t.Fatal("expected transient error to block the request")
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}

	var errResp map[string]string
	body, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error: %v — body: %s", err, body)
	}
	if errResp["error"] != "internal server error" {
		t.Errorf("error = %q, want internal server error", errResp["error"])
	}
}

func TestVerifyRegistered_NilAuthorizerAllowsAll(t *testing.T) {
	// When no UserAuthorizer is configured (e.g. dev mode, no Firestore),
	// all authenticated users should be allowed.
	user := &User{ID: "uid-leon", Email: "leon.black@example-corp.com"}
	rr := httptest.NewRecorder()

	allowed := verifyRegistered(context.Background(), rr, user, nil)
	if !allowed {
		t.Fatal("expected nil authorizer to allow all users")
	}
}

func TestVerifyRegistered_ServiceAccountBlocked(t *testing.T) {
	// verifyRegistered itself blocks unknown service accounts when called
	// directly. In production, the middleware skips verifyRegistered for SAs
	// authenticated via Google ID token (Strategy 2) — but if a SA were to
	// reach verifyRegistered through another path, it would still be rejected.
	authorizer := UserAuthorizer(func(_ context.Context, email string) error {
		// Only known team emails pass.
		known := map[string]bool{
			"larry.david@example-corp.com": true,
			"jeff.greene@example-corp.com": true,
		}
		if known[email] {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrNotRegistered, email)
	})

	sa := &User{ID: "100000000000000000000", Email: "susie-bot@other-project.iam.gserviceaccount.com"}
	rr := httptest.NewRecorder()

	allowed := verifyRegistered(context.Background(), rr, sa, authorizer)
	if allowed {
		t.Fatal("expected external service account to be blocked")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}
