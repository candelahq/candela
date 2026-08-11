package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ErrNotRegistered is the sentinel error that UserAuthorizer should return
// (or wrap) when the email is not found in the user store. Any other error
// is treated as a transient failure and results in a 500 instead of 403.
var ErrNotRegistered = errors.New("user not registered")

// errServiceAccountDenied is the user-facing message returned when a service
// account attempts to authenticate but is not on the allowlist.
const errServiceAccountDenied = "service account not authorized — use personal credentials (candela auth login) or contact your admin to allowlist this SA. " +
	"Check your identity: a service account may be unintentionally used from your environment (ADC, GOOGLE_APPLICATION_CREDENTIALS, etc.)"

// UserAuthorizer checks if an email belongs to a registered user.
// Returns nil if the user exists.
// Returns ErrNotRegistered (or a wrapped form) if the user does not exist.
// Returns any other error for transient failures (database down, timeout, etc.).
// Pass nil to allow all authenticated identities (dev mode, no Firestore).
type UserAuthorizer func(ctx context.Context, email string) error

// FirebaseAuthMiddleware validates Firebase ID tokens (from browser users) and
// Google ID tokens (from candela-local / service accounts).
//
// Auth flow:
//   - Browser: Firebase Auth → ID token in Authorization: Bearer header
//   - candela-local: Cloud Run invoker IAM → Google ID token in Authorization header
//
// If userAuth is non-nil, authenticated users are verified against the user store.
// Only registered users are allowed through; unknown identities receive 403.
//
// In dev mode (devMode=true), no validation is performed; a synthetic admin
// user is injected instead.
// selfServicePaths are ConnectRPC procedures that bypass the registration
// gate. These allow auto-provisioning on first login (GetCurrentUser) and
// self-service budget queries (GetMyBudget) without requiring admin
// pre-provisioning. The handler itself is responsible for auth checks.
var selfServicePaths = map[string]bool{
	"/candela.v1.UserService/GetCurrentUser": true,
	"/candela.v1.UserService/GetMyBudget":    true,
}

// MiddlewareOption configures optional behavior of FirebaseAuthMiddleware.
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	accessTokenValidator AccessTokenValidator
}

// WithAccessTokenValidator overrides the default Google OAuth2 userinfo
// validator used by Strategy 3. Use this in tests to inject a mock and
// avoid live network calls to Google.
func WithAccessTokenValidator(v AccessTokenValidator) MiddlewareOption {
	return func(c *middlewareConfig) { c.accessTokenValidator = v }
}

func FirebaseAuthMiddleware(next http.Handler, fbAuth TokenVerifier, cloudRunAudience string, userAuth UserAuthorizer, devMode bool, allowedSAs []string, opts ...MiddlewareOption) http.Handler {
	cfg := &middlewareConfig{
		accessTokenValidator: DefaultAccessTokenValidator(),
	}
	for _, o := range opts {
		o(cfg)
	}
	saAllowlist := NewServiceAccountAllowlist(allowedSAs)
	if saAllowlist.Len() > 0 {
		slog.Info("🔐 service account allowlist active", "count", saAllowlist.Len())
	} else {
		slog.Info("🔐 service account allowlist empty — all SAs will be denied")
	}
	var resolvers []IdentityResolver
	if devMode {
		resolvers = append(resolvers, NewDevResolver())
	}
	resolvers = append(resolvers, NewFirebaseResolver(fbAuth))
	resolvers = append(resolvers, NewGoogleOIDCResolver(cloudRunAudience, saAllowlist))
	resolvers = append(resolvers, NewGoogleOAuthResolver(cfg.accessTokenValidator, saAllowlist))

	cache := NewIdentityCache(1000, 120*time.Second)
	chain := NewResolverChain(cache, resolvers...)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks (liveness + readiness).
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		// Self-service RPCs bypass the registration gate so that
		// auto-provisioning (GetCurrentUser) works for first-time users.
		// Auth token is still validated — only the user-store lookup is skipped.
		isSelfService := selfServicePaths[r.URL.Path]

		// Extract Bearer token from Authorization header.
		token := extractBearerToken(r)
		if token == "" {
			if devMode {
				// Dev mode without token: inject synthetic admin via chain.
				user := &User{ID: "dev-admin", Email: "admin@localhost", Provider: "dev"}
				next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
				return
			}
			slog.Warn("missing authorization header", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "missing authentication")
			return
		}

		user, err := chain.Resolve(r.Context(), token)
		if err != nil {
			var forbiddenErr *ForbiddenError
			if errors.As(err, &forbiddenErr) {
				slog.Warn("auth forbidden", "error", err, "path", r.URL.Path)
				writeError(w, http.StatusForbidden, forbiddenErr.Error())
				return
			}
			slog.Warn("auth failed", "error", err, "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "invalid authentication token")
			return
		}

		if !isSelfService && !verifyRegistered(r.Context(), w, user, userAuth) {
			return
		}

		slog.Debug("authenticated", "provider", user.Provider, "uid", user.ID, "email", user.Email, "path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
	})
}

// verifyRegistered checks if the authenticated user exists in the user store.
// Returns true if the user is allowed (registered or no store configured).
// Returns false and writes an error response if the user is not registered (403)
// or if the lookup fails with a transient error (500).
func verifyRegistered(ctx context.Context, w http.ResponseWriter, user *User, userAuth UserAuthorizer) bool {
	if userAuth == nil {
		return true // no user store — allow all authenticated users
	}
	if err := userAuth(ctx, user.Email); err != nil {
		if errors.Is(err, ErrNotRegistered) {
			slog.Warn("authenticated but not registered — access denied",
				"email", user.Email, "uid", user.ID)
			writeError(w, http.StatusForbidden, "user not registered — contact your admin")
		} else {
			slog.Error("user authorization check failed — internal error",
				"email", user.Email, "uid", user.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return false
	}
	return true
}

// validateAccessToken validates a Google OAuth2 access token by calling
// Google's userinfo endpoint. Returns user info if the token is valid.
func validateAccessToken(ctx context.Context, accessToken string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned status %d", resp.StatusCode)
	}

	var info struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}
	if info.Email == "" {
		return nil, fmt.Errorf("userinfo missing email")
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo missing sub")
	}

	return &User{
		ID:    info.Sub,
		Email: strings.ToLower(info.Email),
	}, nil
}

// extractBearerToken pulls the token from request headers.
// Checks X-Candela-Auth first (set by candela-local behind IAP, carries the
// user's ADC token), then falls back to Authorization (browser/Firebase clients).
// When behind IAP, Authorization is replaced by IAP's own JWT — so we prefer
// X-Candela-Auth which carries the original user identity token.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("X-Candela-Auth")
	if auth == "" {
		auth = r.Header.Get("Authorization")
		if auth == "" {
			return ""
		}
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// ServiceAccountAllowlist controls which GCP service accounts are permitted
// to authenticate. Deny-by-default: if the list is empty, ALL service
// accounts are rejected. Emails are matched case-insensitively.
type ServiceAccountAllowlist struct {
	allowed map[string]bool
}

// NewServiceAccountAllowlist creates an allowlist from the given emails.
// An empty or nil slice means "deny all service accounts".
func NewServiceAccountAllowlist(emails []string) *ServiceAccountAllowlist {
	m := make(map[string]bool, len(emails))
	for _, e := range emails {
		trimmed := strings.TrimSpace(e)
		if trimmed == "" {
			continue // skip blank entries from YAML formatting
		}
		m[strings.ToLower(trimmed)] = true
	}
	return &ServiceAccountAllowlist{allowed: m}
}

// IsAllowed reports whether the given email is on the allowlist.
// Returns false if the allowlist is empty (deny-by-default).
func (a *ServiceAccountAllowlist) IsAllowed(email string) bool {
	return a.allowed[strings.ToLower(strings.TrimSpace(email))]
}

// Len returns the number of entries in the allowlist.
func (a *ServiceAccountAllowlist) Len() int {
	return len(a.allowed)
}
