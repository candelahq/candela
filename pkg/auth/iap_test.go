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

// fakeIAPValidator is a test double that returns preconfigured email/subject
// without calling Google's idtoken.Validate.
type fakeIAPValidator struct {
	email   string
	subject string
	err     error
}

func (f *fakeIAPValidator) ValidateJWT(_ context.Context, _, _ string) (string, string, error) {
	return f.email, f.subject, f.err
}

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
	handler := IAPMiddleware(echoHandler(), "test-audience", true, nil, nil)
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
	handler := IAPMiddleware(healthHandler, "test-audience", false, nil, nil)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rr.Code)
	}
}

func TestIAPMiddleware_MissingHeader(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", false, nil, nil)
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
	handler := IAPMiddleware(echoHandler(), "test-audience", false, nil, nil)
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("x-goog-iap-jwt-assertion", "invalid.jwt.token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestIAPMiddleware_DevMode_AllPaths(t *testing.T) {
	handler := IAPMiddleware(echoHandler(), "test-audience", true, nil, nil)

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

// TestIAPMiddleware_ProductionPath_Registration tests the real IAPMiddleware
// in production mode (devMode=false) with an injected fake JWT validator.
// This exercises the exact production code path including email normalization,
// self-service bypass, and verifyRegistered.
func TestIAPMiddleware_ProductionPath_Registration(t *testing.T) {
	authorizer := UserAuthorizer(func(_ context.Context, email string) error {
		if email == "registered@example.com" {
			return nil
		}
		return ErrNotRegistered
	})

	saAllowlist := []string{"allowed-sa@my-project.iam.gserviceaccount.com"}

	newHandler := func(validator *fakeIAPValidator) http.Handler {
		return IAPMiddleware(echoHandler(), "test-audience", false, authorizer, saAllowlist,
			WithIAPJWTValidator(validator))
	}

	iapReq := func(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("x-goog-iap-jwt-assertion", "fake-jwt-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("registered user reaches handler", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{email: "registered@example.com", subject: "sub-123"})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
		var body map[string]string
		_ = json.NewDecoder(rr.Body).Decode(&body)
		if body["email"] != "registered@example.com" {
			t.Errorf("email = %q, want registered@example.com", body["email"])
		}
		if body["id"] != "sub-123" {
			t.Errorf("id = %q, want sub-123", body["id"])
		}
	})

	t.Run("unregistered user gets 403", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{email: "stranger@example.com", subject: "sub-456"})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
		}
		var errResp map[string]string
		_ = json.NewDecoder(rr.Body).Decode(&errResp)
		if errResp["error"] != "user not registered — contact your admin" {
			t.Errorf("error = %q, want registration error", errResp["error"])
		}
	})

	t.Run("self-service GetCurrentUser bypasses registration", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{email: "stranger@example.com", subject: "sub-456"})
		rr := iapReq(t, h, "/candela.v1.UserService/GetCurrentUser")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("self-service GetMyBudget bypasses registration", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{email: "stranger@example.com", subject: "sub-456"})
		rr := iapReq(t, h, "/candela.v1.UserService/GetMyBudget")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("allowlisted SA bypasses registration", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{
			email:   "allowed-sa@my-project.iam.gserviceaccount.com",
			subject: "sa-sub",
		})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("non-allowlisted SA gets 403", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{
			email:   "rogue-sa@other-project.iam.gserviceaccount.com",
			subject: "sa-rogue",
		})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("email is lowercased", func(t *testing.T) {
		// Verifies the email normalization that CodeRabbit flagged.
		h := newHandler(&fakeIAPValidator{email: "Registered@Example.COM", subject: "sub-789"})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (case-insensitive match); body = %s", rr.Code, rr.Body.String())
		}
		var body map[string]string
		_ = json.NewDecoder(rr.Body).Decode(&body)
		if body["email"] != "registered@example.com" {
			t.Errorf("email = %q, want lowercased registered@example.com", body["email"])
		}
	})

	t.Run("missing email claim gets 403", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{email: "", subject: "sub-no-email"})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("JWT validation failure gets 401", func(t *testing.T) {
		h := newHandler(&fakeIAPValidator{err: fmt.Errorf("bad token")})
		rr := iapReq(t, h, "/candela.v1.UserService/ListUsers")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body = %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("nil authorizer allows all", func(t *testing.T) {
		// No UserAuthorizer — all authenticated users pass.
		h := IAPMiddleware(echoHandler(), "test-audience", false, nil, nil,
			WithIAPJWTValidator(&fakeIAPValidator{email: "anyone@example.com", subject: "sub-any"}))
		req := httptest.NewRequest("POST", "/candela.v1.UserService/ListUsers", nil)
		req.Header.Set("x-goog-iap-jwt-assertion", "fake-jwt")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
	})
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
