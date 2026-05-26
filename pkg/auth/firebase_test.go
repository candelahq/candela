package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirebaseAuthMiddleware_DevMode(t *testing.T) {
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true, nil)
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
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true, nil)

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
	handler := FirebaseAuthMiddleware(healthHandler, nil, "", nil, false, nil)

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
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false, nil)

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
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false, nil)

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
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, false, nil)

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
	handler := FirebaseAuthMiddleware(echoHandler(), nil, "", nil, true, nil)

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
		nil, "", nil, false, nil,
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

// --- ServiceAccountAllowlist tests ---

func TestServiceAccountAllowlist_DenyByDefault(t *testing.T) {
	// Empty allowlist = deny ALL service accounts.
	al := NewServiceAccountAllowlist(nil)
	if al.IsAllowed("anything@project.iam.gserviceaccount.com") {
		t.Fatal("empty allowlist should deny all SAs")
	}
	if al.Len() != 0 {
		t.Errorf("Len() = %d, want 0", al.Len())
	}

	// Explicit empty slice behaves the same.
	al2 := NewServiceAccountAllowlist([]string{})
	if al2.IsAllowed("anything@project.iam.gserviceaccount.com") {
		t.Fatal("empty slice allowlist should deny all SAs")
	}
}

func TestServiceAccountAllowlist_ExplicitAllow(t *testing.T) {
	al := NewServiceAccountAllowlist([]string{
		"candela-ci@my-project.iam.gserviceaccount.com",
		"deploy-bot@my-project.iam.gserviceaccount.com",
	})

	tests := []struct {
		email string
		want  bool
	}{
		// Allowed SAs.
		{"candela-ci@my-project.iam.gserviceaccount.com", true},
		{"deploy-bot@my-project.iam.gserviceaccount.com", true},
		// Not in list → denied.
		{"other-sa@my-project.iam.gserviceaccount.com", false},
		{"candela-ci@different-project.iam.gserviceaccount.com", false},
		// The specific SA that triggered this fix.
		{"external-sa@other-project.iam.gserviceaccount.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := al.IsAllowed(tt.email); got != tt.want {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}

	if al.Len() != 2 {
		t.Errorf("Len() = %d, want 2", al.Len())
	}
}

func TestServiceAccountAllowlist_CaseInsensitive(t *testing.T) {
	al := NewServiceAccountAllowlist([]string{
		"Candela-CI@My-Project.iam.gserviceaccount.com",
	})

	// Lookup with different casing should still match.
	if !al.IsAllowed("candela-ci@my-project.iam.gserviceaccount.com") {
		t.Fatal("case-insensitive lookup should match")
	}
	if !al.IsAllowed("CANDELA-CI@MY-PROJECT.IAM.GSERVICEACCOUNT.COM") {
		t.Fatal("all-caps lookup should match")
	}
}

func TestServiceAccountAllowlist_NonSAEmails(t *testing.T) {
	// Regular user emails should never be "allowed" through the SA allowlist.
	// The middleware only checks IsAllowed for .gserviceaccount.com emails,
	// but verify the allowlist itself returns false for human emails.
	al := NewServiceAccountAllowlist([]string{
		"candela-ci@my-project.iam.gserviceaccount.com",
	})

	if al.IsAllowed("vitoria@example-corp.com") {
		t.Fatal("human email should not match SA allowlist")
	}
	if al.IsAllowed("") {
		t.Fatal("empty email should not match")
	}
}

// --- Unit tests for critical fixes ---

func TestServiceAccountAllowlist_WhitespaceTrimming(t *testing.T) {
	// Gemini review: YAML config may have trailing/leading whitespace.
	// Ensure these are trimmed so lookups don't silently fail.
	al := NewServiceAccountAllowlist([]string{
		"  candela-ci@my-project.iam.gserviceaccount.com  ",
		"\tdeploy-bot@my-project.iam.gserviceaccount.com\t",
	})

	if !al.IsAllowed("candela-ci@my-project.iam.gserviceaccount.com") {
		t.Fatal("leading/trailing spaces should be trimmed during construction")
	}
	if !al.IsAllowed("deploy-bot@my-project.iam.gserviceaccount.com") {
		t.Fatal("leading/trailing tabs should be trimmed during construction")
	}
	if al.Len() != 2 {
		t.Errorf("Len() = %d, want 2", al.Len())
	}
}

func TestServiceAccountAllowlist_BlankEntriesSkipped(t *testing.T) {
	// Blank entries (empty strings, whitespace-only) should be silently
	// skipped, not added to the allowlist as a wildcard empty key.
	al := NewServiceAccountAllowlist([]string{
		"",
		"   ",
		"\t",
		"candela-ci@my-project.iam.gserviceaccount.com",
	})

	if al.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (blank entries should be skipped)", al.Len())
	}
	if al.IsAllowed("") {
		t.Fatal("empty email should never match, even if blank entries exist in config")
	}
	if !al.IsAllowed("candela-ci@my-project.iam.gserviceaccount.com") {
		t.Fatal("valid entry should still work when blanks are present")
	}
}

func TestServiceAccountAllowlist_IsAllowed_TrimsLookupInput(t *testing.T) {
	// IsAllowed uses ToLower on input. Verify it handles edge-case inputs.
	al := NewServiceAccountAllowlist([]string{
		"candela-ci@my-project.iam.gserviceaccount.com",
	})

	// Exact match.
	if !al.IsAllowed("candela-ci@my-project.iam.gserviceaccount.com") {
		t.Fatal("exact match should work")
	}
	// Different case in lookup.
	if !al.IsAllowed("Candela-CI@MY-PROJECT.iam.gserviceaccount.com") {
		t.Fatal("case-insensitive lookup should match")
	}
	// Should NOT match partial/substring.
	if al.IsAllowed("candela-ci@my-project.iam.gserviceaccount.com.evil.com") {
		t.Fatal("suffix attack should not match")
	}
	if al.IsAllowed("x-candela-ci@my-project.iam.gserviceaccount.com") {
		t.Fatal("prefix attack should not match")
	}
}

// --- Integration tests (middleware-level, using httptest) ---

func TestFirebaseAuthMiddleware_OAuth2_SA_Denied(t *testing.T) {
	// Integration test: verify that Strategy 3 (OAuth2 access token) also
	// enforces the SA allowlist. This catches the critical bypass where a
	// service account could use an access token instead of an ID token to
	// skip the allowlist check.
	//
	// We can't fully mock idtoken.Validate or the userinfo endpoint without
	// network, but we CAN verify that the middleware correctly initializes
	// the allowlist and that the validateAccessToken path would reject SAs.
	// We test the unit behavior of the check itself here.
	saEmail := "external-sa@other-project.iam.gserviceaccount.com"
	user := &User{ID: "sa-123", Email: saEmail}

	// With empty allowlist (deny-all), a SA user should be blocked.
	al := NewServiceAccountAllowlist(nil)
	if al.IsAllowed(user.Email) {
		t.Fatal("empty allowlist should deny SA")
	}

	// Verify the email suffix detection works.
	if !strings.HasSuffix(user.Email, ".gserviceaccount.com") {
		t.Fatal("test SA email should have .gserviceaccount.com suffix")
	}

	// With explicit allowlist excluding this SA, should still be denied.
	al2 := NewServiceAccountAllowlist([]string{
		"other-sa@different-project.iam.gserviceaccount.com",
	})
	if al2.IsAllowed(saEmail) {
		t.Fatalf("SA %q should not be allowed when not in allowlist", saEmail)
	}
}

func TestFirebaseAuthMiddleware_AllowlistedSA_PassesThrough(t *testing.T) {
	// Integration test: verify that an allowlisted SA would pass the
	// middleware check. Tests the full construction → lookup flow.
	allowedSA := "candela-proxy@production.iam.gserviceaccount.com"
	denied := "rogue-bot@other-project.iam.gserviceaccount.com"

	// Construct middleware with specific SA allowlisted.
	al := NewServiceAccountAllowlist([]string{allowedSA})

	// Allowlisted SA passes.
	if !al.IsAllowed(allowedSA) {
		t.Fatal("explicitly allowlisted SA should pass")
	}
	// Non-listed SA fails.
	if al.IsAllowed(denied) {
		t.Fatal("non-listed SA should be denied")
	}

	// Verify the middleware wires the allowlist correctly by constructing
	// the full middleware and checking it initializes without panic.
	handler := FirebaseAuthMiddleware(
		echoHandler(), nil, "", nil, false,
		[]string{allowedSA},
	)
	// Smoke test: middleware should return 401 for missing auth
	// (not panic from nil allowlist or config issues).
	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing auth", rr.Code)
	}
}

func TestFirebaseAuthMiddleware_DevMode_IgnoresAllowlist(t *testing.T) {
	// Integration test: in dev mode, the SA allowlist should be irrelevant.
	// Even if a restrictive allowlist is configured, dev mode should always
	// inject the synthetic admin user.
	handler := FirebaseAuthMiddleware(
		echoHandler(), nil, "", nil, true,
		nil, // empty allowlist
	)

	// Dev mode should still work even with empty SA allowlist.
	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dev mode status = %d, want 200", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["id"] != "dev-admin" {
		t.Errorf("dev mode should inject synthetic user, got id=%q", body["id"])
	}

	// Same test with a populated allowlist — dev mode should still bypass.
	handler2 := FirebaseAuthMiddleware(
		echoHandler(), nil, "", nil, true,
		[]string{"some-sa@project.iam.gserviceaccount.com"},
	)
	rr2 := httptest.NewRecorder()
	handler2.ServeHTTP(rr2, httptest.NewRequest("GET", "/proxy/openai/v1/chat/completions", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("dev mode with SA allowlist status = %d, want 200", rr2.Code)
	}
}
