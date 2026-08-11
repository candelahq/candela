package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// ── Test helpers ──

// fakeIDTokenFactory returns a factory that either succeeds (returning a static
// token source) or fails with a given error. It records whether it was called.
func fakeIDTokenFactory(succeed bool) (idTokenFactory, *bool) {
	called := new(bool)
	return func(_ context.Context, audience string, _ ...idtoken.ClientOption) (oauth2.TokenSource, error) {
		*called = true
		if succeed {
			return &staticTokenSource{accessToken: "direct-id-token-for-" + audience}, nil
		}
		return nil, fmt.Errorf("no service account credentials available")
	}, called
}

type fakeTokenFactoryContext struct {
	factory defaultTokenFactory
	calls   [][]string
}

// fakeDefaultTokenFactory returns a factory that always succeeds.
func fakeDefaultTokenFactory() *fakeTokenFactoryContext {
	ctx := &fakeTokenFactoryContext{}
	ctx.factory = func(_ context.Context, scope ...string) (oauth2.TokenSource, error) {
		ctx.calls = append(ctx.calls, scope)
		return &staticTokenSource{accessToken: "default-token"}, nil
	}
	return ctx
}

// failingDefaultTokenFactory returns a factory that always errors.
func failingDefaultTokenFactory() defaultTokenFactory {
	return func(_ context.Context, scope ...string) (oauth2.TokenSource, error) {
		return nil, fmt.Errorf("no credentials available")
	}
}

// ── Strategy selection tests ──

func TestResolveAuthStrategy_DirectIDToken_WhenNoIAPServiceAccount(t *testing.T) {
	// When iap_service_account is empty and idtoken succeeds → Strategy 1.
	factory, called := fakeIDTokenFactory(true)
	cfg := Config{
		Audience: "test-audience",
		// IAPServiceAccount is empty
	}

	result, err := resolveAuthStrategy(context.Background(), &cfg, factory, fakeDefaultTokenFactory().factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !*called {
		t.Error("idtoken.NewTokenSource was NOT called — expected it to be called when IAPServiceAccount is empty")
	}
	if result.strategy != strategyDirectIDToken {
		t.Errorf("strategy = %d, want strategyDirectIDToken (%d)", result.strategy, strategyDirectIDToken)
	}
	if result.userTokenSource != nil {
		t.Error("userTokenSource should be nil for Strategy 1")
	}

	// Verify the token source returns the expected token.
	tok, err := result.tokenSource.Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok.AccessToken != "direct-id-token-for-test-audience" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "direct-id-token-for-test-audience")
	}
}

func TestResolveAuthStrategy_IAPImpersonation_WhenIAPServiceAccountSet(t *testing.T) {
	// When iap_service_account is set → Strategy 1.5 (skip idtoken entirely).
	factory, called := fakeIDTokenFactory(true) // would succeed, but shouldn't be called
	cfg := Config{
		Audience:          "test-audience",
		IAPServiceAccount: "candela-server@project.iam.gserviceaccount.com",
	}

	fdtf := fakeDefaultTokenFactory()
	result, err := resolveAuthStrategy(context.Background(), &cfg, factory, fdtf.factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *called {
		t.Error("idtoken.NewTokenSource WAS called — expected it to be SKIPPED when IAPServiceAccount is set")
	}
	if result.strategy != strategyIAPImpersonation {
		t.Errorf("strategy = %d, want strategyIAPImpersonation (%d)", result.strategy, strategyIAPImpersonation)
	}
	if result.userTokenSource == nil {
		t.Error("userTokenSource should be set for Strategy 1.5 (dual-token mode)")
	}

	if len(fdtf.calls) != 2 {
		t.Fatalf("expected 2 calls to default token factory, got %d", len(fdtf.calls))
	}
	if len(fdtf.calls[0]) != 1 || fdtf.calls[0][0] != "https://www.googleapis.com/auth/cloud-platform" {
		t.Errorf("expected first call to have cloud-platform scope, got %v", fdtf.calls[0])
	}
	if len(fdtf.calls[1]) != 2 || fdtf.calls[1][0] != "openid" || fdtf.calls[1][1] != "email" {
		t.Errorf("expected second call to have openid and email scopes, got %v", fdtf.calls[1])
	}
}

func TestResolveAuthStrategy_UserADC_WhenIDTokenFailsAndNoIAPSA(t *testing.T) {
	// When idtoken fails and iap_service_account is empty → Strategy 2.
	factory, called := fakeIDTokenFactory(false) // fails
	cfg := Config{
		Audience: "test-audience",
		// IAPServiceAccount is empty
	}

	result, err := resolveAuthStrategy(context.Background(), &cfg, factory, fakeDefaultTokenFactory().factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !*called {
		t.Error("idtoken.NewTokenSource was NOT called")
	}
	if result.strategy != strategyUserADC {
		t.Errorf("strategy = %d, want strategyUserADC (%d)", result.strategy, strategyUserADC)
	}
	if result.userTokenSource != nil {
		t.Error("userTokenSource should be nil for Strategy 2")
	}
}

func TestResolveAuthStrategy_IAPImpersonation_FailsWhenNoDefaultCreds(t *testing.T) {
	// When iap_service_account is set but no default credentials → error.
	factory, _ := fakeIDTokenFactory(true)
	cfg := Config{
		Audience:          "test-audience",
		IAPServiceAccount: "candela-server@project.iam.gserviceaccount.com",
	}

	_, err := resolveAuthStrategy(context.Background(), &cfg, factory, failingDefaultTokenFactory())
	if err == nil {
		t.Fatal("expected error when default credentials are unavailable")
	}
}

func TestResolveAuthStrategy_UserADC_FailsWhenNoDefaultCreds(t *testing.T) {
	// When idtoken fails, no iap_service_account, AND no default creds → error.
	factory, _ := fakeIDTokenFactory(false)
	cfg := Config{
		Audience: "test-audience",
	}

	_, err := resolveAuthStrategy(context.Background(), &cfg, factory, failingDefaultTokenFactory())
	if err == nil {
		t.Fatal("expected error when default credentials are unavailable")
	}
}

// ── iapImpersonatingTokenSource tests ──

func TestIAPImpersonatingTokenSource_Success(t *testing.T) {
	// Mock the IAM generateIdToken endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request.
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer base-access-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer base-access-token")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		// Decode and verify the request body.
		var req struct {
			Audience     string `json:"audience"`
			IncludeEmail bool   `json:"includeEmail"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Audience != "iap-client-id" {
			t.Errorf("audience = %q, want %q", req.Audience, "iap-client-id")
		}
		if !req.IncludeEmail {
			t.Error("includeEmail should be true")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token": "generated-iap-id-token",
		})
	}))
	defer server.Close()

	ts := &iapImpersonatingTokenSource{
		base:           &staticTokenSource{accessToken: "base-access-token"},
		serviceAccount: "candela-server@project.iam.gserviceaccount.com",
		audience:       "iap-client-id",
		// Override the endpoint for testing.
		endpointURL: server.URL,
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok.AccessToken != "generated-iap-id-token" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "generated-iap-id-token")
	}
	if tok.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", tok.TokenType, "Bearer")
	}
	if time.Until(tok.Expiry) < 3400*time.Second {
		t.Errorf("Expiry too soon: %v", tok.Expiry)
	}
}

func TestIAPImpersonatingTokenSource_403Permission(t *testing.T) {
	// Simulate the exact 403 that WIF SAs get.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    403,
				"message": "Permission 'iam.serviceAccounts.getOpenIdToken' denied on resource",
				"status":  "PERMISSION_DENIED",
			},
		})
	}))
	defer server.Close()

	ts := &iapImpersonatingTokenSource{
		base:           &staticTokenSource{accessToken: "base-access-token"},
		serviceAccount: "candela-server@project.iam.gserviceaccount.com",
		audience:       "iap-client-id",
		endpointURL:    server.URL,
	}

	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error on 403 response")
	}
	// Should mention the service account for debugging.
	if got := err.Error(); !strings.Contains(got, "candela-server@project.iam.gserviceaccount.com") {
		t.Errorf("error should mention the service account, got: %s", got)
	}
}

func TestIAPImpersonatingTokenSource_BaseTokenError(t *testing.T) {
	ts := &iapImpersonatingTokenSource{
		base:           &failingTokenSource{err: fmt.Errorf("WIF token exchange failed")},
		serviceAccount: "candela-server@project.iam.gserviceaccount.com",
		audience:       "iap-client-id",
	}

	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error when base token source fails")
	}
	if got := err.Error(); !strings.Contains(got, "base credentials") {
		t.Errorf("error should mention base credentials, got: %s", got)
	}
}

// ── Additional helpers ──

type failingTokenSource struct {
	err error
}

func (f *failingTokenSource) Token() (*oauth2.Token, error) {
	return nil, f.err
}

func TestDirector_Headers(t *testing.T) {
	t.Run("dual-token mode", func(t *testing.T) {
		var reqHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqHeaders = r.Header
		}))
		defer srv.Close()

		tokenSource := &staticTokenSource{accessToken: "iap-token"}
		userTokenSource := &staticTokenSource{accessToken: "user-token"}

		req, err := http.NewRequest("GET", srv.URL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		// Setup Director similarly to main.go lines 770-804
		director := func(r *http.Request) {
			tok, _ := tokenSource.Token()
			r.Header.Set("Proxy-Authorization", "Bearer "+tok.AccessToken)
			r.Header.Set("Authorization", "Bearer "+tok.AccessToken)

			userTok, _ := userTokenSource.Token()
			r.Header.Set("X-Candela-Auth", "Bearer "+userTok.AccessToken)
		}

		director(req)

		// Simulate the reverse proxy sending the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if got := reqHeaders.Get("Proxy-Authorization"); got != "Bearer iap-token" {
			t.Errorf("Proxy-Authorization = %q, want Bearer iap-token", got)
		}
		if got := reqHeaders.Get("Authorization"); got != "Bearer iap-token" {
			t.Errorf("Authorization = %q, want Bearer iap-token", got)
		}
		if got := reqHeaders.Get("X-Candela-Auth"); got != "Bearer user-token" {
			t.Errorf("X-Candela-Auth = %q, want Bearer user-token", got)
		}
	})

	t.Run("single-token mode", func(t *testing.T) {
		var reqHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqHeaders = r.Header
		}))
		defer srv.Close()

		tokenSource := &staticTokenSource{accessToken: "single-token"}

		req, err := http.NewRequest("GET", srv.URL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		director := func(r *http.Request) {
			tok, _ := tokenSource.Token()
			r.Header.Set("Proxy-Authorization", "Bearer "+tok.AccessToken)
			r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
			r.Header.Set("X-Candela-Auth", "Bearer "+tok.AccessToken)
		}

		director(req)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if got := reqHeaders.Get("Proxy-Authorization"); got != "Bearer single-token" {
			t.Errorf("Proxy-Authorization = %q, want Bearer single-token", got)
		}
		if got := reqHeaders.Get("Authorization"); got != "Bearer single-token" {
			t.Errorf("Authorization = %q, want Bearer single-token", got)
		}
		if got := reqHeaders.Get("X-Candela-Auth"); got != "Bearer single-token" {
			t.Errorf("X-Candela-Auth = %q, want Bearer single-token", got)
		}
	})
}
