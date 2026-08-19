package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"
)

// IAPJWTValidator validates IAP JWT assertions and returns the email and
// subject claims. This interface abstracts idtoken.Validate for testability.
//
// Implementations:
//   - googleIAPValidator (production, calls idtoken.Validate)
//   - fakeIAPValidator (tests)
type IAPJWTValidator interface {
	ValidateJWT(ctx context.Context, assertion, audience string) (email, subject string, err error)
}

// googleIAPValidator is the production implementation using Google's idtoken package.
type googleIAPValidator struct{}

func (g *googleIAPValidator) ValidateJWT(ctx context.Context, assertion, audience string) (string, string, error) {
	payload, err := idtoken.Validate(ctx, assertion, audience)
	if err != nil {
		return "", "", err
	}
	email, _ := payload.Claims["email"].(string)
	return email, payload.Subject, nil
}

// IAPOption configures optional behavior of IAPMiddleware.
type IAPOption func(*iapConfig)

type iapConfig struct {
	jwtValidator IAPJWTValidator
}

// WithIAPJWTValidator overrides the default Google IAP JWT validator.
// Use this in tests to inject a fake and avoid live calls to Google.
func WithIAPJWTValidator(v IAPJWTValidator) IAPOption {
	return func(c *iapConfig) { c.jwtValidator = v }
}

// IAPMiddleware validates the IAP JWT assertion header on every request and
// injects the authenticated user into the request context.
//
// In dev mode (devMode=true), no JWT validation is performed; a synthetic
// admin user is injected instead.
//
// If userAuth is non-nil, authenticated users are verified against the user
// store. Only registered users (or allowlisted service accounts) are allowed
// through; unknown identities receive 403. Self-service RPCs
// (GetCurrentUser, GetMyBudget) bypass the registration gate so that
// auto-provisioning works on first login.
//
// Header: x-goog-iap-jwt-assertion (set by IAP automatically)
func IAPMiddleware(next http.Handler, audience string, devMode bool, userAuth UserAuthorizer, allowedSAs []string, opts ...IAPOption) http.Handler {
	cfg := &iapConfig{
		jwtValidator: &googleIAPValidator{},
	}
	for _, o := range opts {
		o(cfg)
	}

	saAllowlist := NewServiceAccountAllowlist(allowedSAs)
	if saAllowlist.Len() > 0 {
		slog.Info("🔐 IAP: service account allowlist active", "count", saAllowlist.Len())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks (liveness + readiness).
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		if devMode {
			// Dev mode: inject a synthetic admin user.
			user := &User{
				ID:    "dev-admin",
				Email: "admin@localhost",
			}
			next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
			return
		}

		// Production: validate IAP JWT.
		assertion := r.Header.Get("x-goog-iap-jwt-assertion")
		if assertion == "" {
			slog.Warn("missing IAP JWT assertion", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "missing authentication")
			return
		}

		email, sub, err := cfg.jwtValidator.ValidateJWT(r.Context(), assertion, audience)
		if err != nil {
			slog.Warn("invalid IAP JWT", "error", err, "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "invalid authentication token")
			return
		}

		if sub == "" {
			sub = email // Fallback to email as ID.
		}

		if email == "" {
			slog.Warn("IAP JWT missing email claim", "sub", sub, "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "identity missing email claim")
			return
		}

		user := &User{
			ID:    sub,
			Email: strings.ToLower(email),
		}

		// Verify user is registered (unless self-service RPC).
		isSelfService := selfServicePaths[r.URL.Path]
		if !isSelfService && !verifyRegistered(r.Context(), w, user, userAuth, saAllowlist) {
			return
		}

		slog.Debug("authenticated request",
			"user_id", user.ID,
			"email", user.Email,
			"path", r.URL.Path)

		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
	})
}

// writeError sends a JSON error response.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
		"code":  fmt.Sprintf("%d", code),
	})
}
