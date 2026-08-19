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

// TestIAPRegistration_IntegrationHTTP tests the full IAPMiddleware HTTP chain
// in production mode (devMode=false) with an injected JWT validator. This
// exercises the exact production code path: JWT validation → email
// normalization → self-service check → verifyRegistered → context injection.
func TestIAPRegistration_IntegrationHTTP(t *testing.T) {
	authorizer := UserAuthorizer(func(_ context.Context, email string) error {
		if email == "registered@example.com" {
			return nil
		}
		return ErrNotRegistered
	})

	saAllowlist := []string{"allowed-sa@my-project.iam.gserviceaccount.com"}

	// Build a real IAPMiddleware with an injected fake validator.
	// The inner handler echoes the authenticated user back as JSON.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := FromContext(r.Context())
		if user == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"no user in context"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"email": user.Email,
			"id":    user.ID,
		})
	})

	// fakeValidator is shared — each test sets the assertion header to signal
	// identity. We use a per-request validator that reads the assertion.
	perRequestValidator := &routingIAPValidator{
		identities: map[string]fakeIdentity{
			"registered-jwt":   {email: "registered@example.com", subject: "sub-reg"},
			"unregistered-jwt": {email: "stranger@example.com", subject: "sub-stranger"},
			"mixed-case-jwt":   {email: "Registered@Example.COM", subject: "sub-mixed"},
			"allowed-sa-jwt":   {email: "allowed-sa@my-project.iam.gserviceaccount.com", subject: "sa-allowed"},
			"rogue-sa-jwt":     {email: "rogue-sa@other-project.iam.gserviceaccount.com", subject: "sa-rogue"},
			"no-email-jwt":     {email: "", subject: "sub-no-email"},
			"self-service-jwt": {email: "stranger@example.com", subject: "sub-self"},
		},
	}

	handler := IAPMiddleware(inner, "test-audience", false, authorizer, saAllowlist,
		WithIAPJWTValidator(perRequestValidator))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	doReq := func(t *testing.T, path, jwt string) (int, string) {
		t.Helper()
		req, err := http.NewRequest("POST", server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if jwt != "" {
			req.Header.Set("x-goog-iap-jwt-assertion", jwt)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	t.Run("registered user reaches handler", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/ListUsers", "registered-jwt")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", code, body)
		}
		var resp map[string]string
		_ = json.Unmarshal([]byte(body), &resp)
		if resp["email"] != "registered@example.com" {
			t.Errorf("email = %q, want registered@example.com", resp["email"])
		}
	})

	t.Run("unregistered user gets 403", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/ListUsers", "unregistered-jwt")
		if code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %s", code, body)
		}
		var errResp map[string]string
		_ = json.Unmarshal([]byte(body), &errResp)
		if errResp["error"] != "user not registered — contact your admin" {
			t.Errorf("error = %q, want registration error", errResp["error"])
		}
	})

	t.Run("missing JWT gets 401", func(t *testing.T) {
		code, _ := doReq(t, "/candela.v1.UserService/ListUsers", "")
		if code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})

	t.Run("self-service GetCurrentUser bypasses registration", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/GetCurrentUser", "self-service-jwt")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", code, body)
		}
	})

	t.Run("self-service GetMyBudget bypasses registration", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/GetMyBudget", "self-service-jwt")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", code, body)
		}
	})

	t.Run("allowlisted SA bypasses registration", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/ListUsers", "allowed-sa-jwt")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", code, body)
		}
	})

	t.Run("non-allowlisted SA gets 403", func(t *testing.T) {
		code, _ := doReq(t, "/candela.v1.UserService/ListUsers", "rogue-sa-jwt")
		if code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("email is lowercased through production path", func(t *testing.T) {
		code, body := doReq(t, "/candela.v1.UserService/ListUsers", "mixed-case-jwt")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", code, body)
		}
		var resp map[string]string
		_ = json.Unmarshal([]byte(body), &resp)
		if resp["email"] != "registered@example.com" {
			t.Errorf("email = %q, want lowercased registered@example.com", resp["email"])
		}
	})

	t.Run("missing email claim gets 403", func(t *testing.T) {
		code, _ := doReq(t, "/candela.v1.UserService/ListUsers", "no-email-jwt")
		if code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("healthz bypasses everything", func(t *testing.T) {
		code, _ := doReq(t, "/healthz", "")
		// Health check goes straight to inner handler without auth.
		// Inner handler returns 500 (no user in context) — the point is
		// it doesn't return 401/403.
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Fatalf("healthz should bypass auth, got %d", code)
		}
	})

	t.Run("readyz bypasses everything", func(t *testing.T) {
		code, _ := doReq(t, "/readyz", "")
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Fatalf("readyz should bypass auth, got %d", code)
		}
	})
}

// routingIAPValidator maps JWT assertion strings to fake identities,
// allowing a single middleware instance to serve multiple test scenarios.
type routingIAPValidator struct {
	identities map[string]fakeIdentity
}

type fakeIdentity struct {
	email   string
	subject string
}

func (r *routingIAPValidator) ValidateJWT(_ context.Context, assertion, _ string) (string, string, error) {
	id, ok := r.identities[assertion]
	if !ok {
		return "", "", fmt.Errorf("unknown JWT assertion in test: %s", assertion)
	}
	return id.email, id.subject, nil
}
