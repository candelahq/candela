package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fbauth "firebase.google.com/go/v4/auth"
)

// mockTokenVerifier is a configurable mock for TokenVerifier.
type mockTokenVerifier struct {
	// verifyFunc is called by VerifyIDToken. Set this per-test.
	verifyFunc func(ctx context.Context, idToken string) (*fbauth.Token, error)
}

func (m *mockTokenVerifier) VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error) {
	return m.verifyFunc(ctx, idToken)
}

// newTestMiddleware builds a minimal auth middleware with the given TokenVerifier
// and optional UserAuthorizer. Returns an httptest.Server ready for requests.
func newTestMiddleware(t *testing.T, verifier TokenVerifier, userAuth UserAuthorizer, allowedSAs []string) *httptest.Server {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := FromContext(r.Context())
		if user == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"no user in context"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"email":"%s","id":"%s"}`, user.Email, user.ID)
	})
	handler := FirebaseAuthMiddleware(inner, verifier, "", userAuth, false, allowedSAs)
	return httptest.NewServer(handler)
}

func doRequest(t *testing.T, url, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", url+"/candela.v1.UserService/ListUsers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestTokenVerifier_ValidFirebaseToken(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, token string) (*fbauth.Token, error) {
			if token == "valid-firebase-token" {
				return &fbauth.Token{
					UID:    "firebase-uid-123",
					Claims: map[string]interface{}{"email": "user@example.com"},
				}, nil
			}
			return nil, fmt.Errorf("invalid token")
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, body := doRequest(t, srv.URL, "valid-firebase-token")
	if status != 200 {
		t.Fatalf("status = %d, want 200; body = %s", status, body)
	}
	if !strings.Contains(body, `"email":"user@example.com"`) {
		t.Errorf("body = %s, want email user@example.com", body)
	}
	if !strings.Contains(body, `"id":"firebase-uid-123"`) {
		t.Errorf("body = %s, want id firebase-uid-123", body)
	}
}

func TestTokenVerifier_InvalidToken_Returns401(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return nil, fmt.Errorf("token expired")
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, _ := doRequest(t, srv.URL, "expired-token")
	if status != 401 {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestTokenVerifier_MissingToken_Returns401(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, _ := doRequest(t, srv.URL, "")
	if status != 401 {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestTokenVerifier_MissingEmailClaim_Returns401(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return &fbauth.Token{
				UID:    "uid-no-email",
				Claims: map[string]interface{}{}, // no email claim
			}, nil
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, _ := doRequest(t, srv.URL, "token-no-email")
	if status != 401 {
		t.Fatalf("status = %d, want 401 (missing email claim)", status)
	}
}

func TestTokenVerifier_ValidToken_UnregisteredUser_Returns403(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return &fbauth.Token{
				UID:    "uid-unknown",
				Claims: map[string]interface{}{"email": "unknown@example.com"},
			}, nil
		},
	}

	userAuth := func(_ context.Context, email string) error {
		return fmt.Errorf("%w: %s", ErrNotRegistered, email)
	}

	srv := newTestMiddleware(t, verifier, userAuth, nil)
	defer srv.Close()

	status, _ := doRequest(t, srv.URL, "valid-but-unregistered")
	if status != 403 {
		t.Fatalf("status = %d, want 403 (unregistered user)", status)
	}
}

func TestTokenVerifier_ValidToken_RegisteredUser_Returns200(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return &fbauth.Token{
				UID:    "uid-registered",
				Claims: map[string]interface{}{"email": "registered@example.com"},
			}, nil
		},
	}

	userAuth := func(_ context.Context, email string) error {
		if email == "registered@example.com" {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrNotRegistered, email)
	}

	srv := newTestMiddleware(t, verifier, userAuth, nil)
	defer srv.Close()

	status, body := doRequest(t, srv.URL, "valid-registered")
	if status != 200 {
		t.Fatalf("status = %d, want 200; body = %s", status, body)
	}
}

func TestTokenVerifier_SelfServicePath_BypassesRegistration(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return &fbauth.Token{
				UID:    "uid-new",
				Claims: map[string]interface{}{"email": "newuser@example.com"},
			}, nil
		},
	}

	userAuth := func(_ context.Context, _ string) error {
		return fmt.Errorf("%w: not found", ErrNotRegistered)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := FromContext(r.Context())
		if user == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"email":"%s"}`, user.Email)
	})
	handler := FirebaseAuthMiddleware(inner, verifier, "", userAuth, false, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GetCurrentUser is a self-service path — bypasses registration.
	req, _ := http.NewRequest("POST", srv.URL+"/candela.v1.UserService/GetCurrentUser", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (self-service bypasses registration); body = %s", resp.StatusCode, body)
	}
}

func TestTokenVerifier_HealthCheckBypassesAuth(t *testing.T) {
	// No verifier, no user auth — health checks should still work.
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := FirebaseAuthMiddleware(inner, nil, "", nil, false, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 for /healthz", resp.StatusCode)
	}
}

func TestTokenVerifier_Cascade_FirebaseFails_FallsThrough(t *testing.T) {
	// Firebase fails, but OAuth2 token validation is Strategy 3.
	// Since we can't easily mock the OAuth2 path (it calls Google),
	// we verify that when Firebase fails and no other strategies work,
	// we get 401 (not a panic or 500).
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return nil, fmt.Errorf("Firebase: invalid token")
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, _ := doRequest(t, srv.URL, "not-a-firebase-token")
	if status != 401 {
		t.Fatalf("status = %d, want 401 (all strategies fail)", status)
	}
}

func TestTokenVerifier_EmailNormalization(t *testing.T) {
	verifier := &mockTokenVerifier{
		verifyFunc: func(_ context.Context, _ string) (*fbauth.Token, error) {
			return &fbauth.Token{
				UID:    "uid-mixed",
				Claims: map[string]interface{}{"email": "Admin@Example.COM"},
			}, nil
		},
	}

	srv := newTestMiddleware(t, verifier, nil, nil)
	defer srv.Close()

	status, body := doRequest(t, srv.URL, "valid-mixed-case")
	if status != 200 {
		t.Fatalf("status = %d, want 200; body = %s", status, body)
	}
	if !strings.Contains(body, `"email":"admin@example.com"`) {
		t.Errorf("email not normalized to lowercase: %s", body)
	}
}
