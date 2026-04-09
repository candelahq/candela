package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// echoHandler is a test handler that returns the user from context as JSON.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := FromContext(r.Context())
		if user == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"user":null}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":    user.ID,
			"email": user.Email,
		})
	})
}

func TestIAPMiddleware_DevMode(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", true)
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

func TestIAPMiddleware_HealthCheckBypassesAuth(t *testing.T) {
	// Even in production mode (devMode=false), /healthz should pass without auth.
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := IAPMiddleware(healthHandler, "test-audience", false)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rr.Code)
	}
}

func TestIAPMiddleware_MissingHeader(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", false)
	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}

	body, _ := io.ReadAll(rr.Body)
	var errResp map[string]string
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to decode error body: %v — body: %s", err, body)
	}
	if errResp["error"] != "missing authentication" {
		t.Errorf("error = %q, want %q", errResp["error"], "missing authentication")
	}
}

func TestIAPMiddleware_InvalidJWT(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", false)
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("x-goog-iap-jwt-assertion", "invalid.jwt.token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestIAPMiddleware_DevMode_AllPaths(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", true)

	paths := []string{"/api/data", "/proxy/openai/v1/chat/completions", "/candela.v1.UserService/GetCurrentUser"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 for path %s", rr.Code, path)
			}

			var body map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&body)
			if body["id"] != "dev-admin" {
				t.Errorf("expected dev-admin user for path %s", path)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusForbidden, "access denied")

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["error"] != "access denied" {
		t.Errorf("error = %q, want %q", body["error"], "access denied")
	}
	if body["code"] != "403" {
		t.Errorf("code = %q, want %q", body["code"], "403")
	}
}
