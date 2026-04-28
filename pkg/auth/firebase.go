package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/idtoken"
)

// UserAuthorizer checks if an email belongs to a registered user.
// Returns nil if the user exists, or an error if not found / lookup fails.
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
func FirebaseAuthMiddleware(next http.Handler, fbAuth *fbauth.Client, cloudRunAudience string, userAuth UserAuthorizer, devMode bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks.
		if r.URL.Path == "/healthz" {
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

		// Extract Bearer token from Authorization header.
		token := extractBearerToken(r)
		if token == "" {
			slog.Warn("missing authorization header", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "missing authentication")
			return
		}

		// Strategy 1: Try Firebase ID token (browser users).
		if fbAuth != nil {
			decoded, err := fbAuth.VerifyIDToken(r.Context(), token)
			if err == nil {
				email, ok := decoded.Claims["email"].(string)
				if !ok || email == "" {
					slog.Warn("Firebase token missing email claim", "uid", decoded.UID, "path", r.URL.Path)
					writeError(w, http.StatusUnauthorized, "token missing email claim")
					return
				}
				user := &User{
					ID:    decoded.UID,
					Email: strings.ToLower(email),
				}
				if !verifyRegistered(r.Context(), w, user, userAuth) {
					return
				}
				slog.Debug("authenticated via Firebase",
					"uid", user.ID, "email", user.Email, "path", r.URL.Path)
				next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
				return
			}
			slog.Debug("Firebase token validation failed, trying Google ID token",
				"error", err, "path", r.URL.Path)
		}

		// Strategy 2: Try Google ID token (candela-local / service accounts).
		if cloudRunAudience != "" {
			payload, err := idtoken.Validate(r.Context(), token, cloudRunAudience)
			if err == nil {
				email, ok := payload.Claims["email"].(string)
				if !ok || email == "" {
					slog.Warn("Google ID token missing email claim", "sub", payload.Subject, "path", r.URL.Path)
					writeError(w, http.StatusUnauthorized, "token missing email claim")
					return
				}
				if payload.Subject == "" {
					slog.Warn("Google ID token missing sub claim", "email", email, "path", r.URL.Path)
					writeError(w, http.StatusUnauthorized, "token missing subject claim")
					return
				}
				user := &User{
					ID:    payload.Subject,
					Email: strings.ToLower(email),
				}
				if !verifyRegistered(r.Context(), w, user, userAuth) {
					return
				}
				slog.Debug("authenticated via Google ID token",
					"uid", user.ID, "email", user.Email, "path", r.URL.Path)
				next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
				return
			}
			slog.Debug("Google ID token validation failed, trying OAuth2 access token",
				"error", err, "path", r.URL.Path)
		}

		// Strategy 3: Try Google OAuth2 access token (candela-local with user ADC).
		// Validates via Google's userinfo endpoint.
		user, err := validateAccessToken(r.Context(), token)
		if err == nil {
			if !verifyRegistered(r.Context(), w, user, userAuth) {
				return
			}
			slog.Debug("authenticated via OAuth2 access token",
				"uid", user.ID, "email", user.Email, "path", r.URL.Path)
			next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
			return
		}
		slog.Warn("all auth strategies failed", "path", r.URL.Path, "lastError", err)

		writeError(w, http.StatusUnauthorized, "invalid authentication token")
	})
}

// verifyRegistered checks if the authenticated user exists in the user store.
// Returns true if the user is allowed (registered or no store configured).
// Returns false and writes a 403 response if the user is not registered.
func verifyRegistered(ctx context.Context, w http.ResponseWriter, user *User, userAuth UserAuthorizer) bool {
	if userAuth == nil {
		return true // no user store — allow all authenticated users
	}
	if err := userAuth(ctx, user.Email); err != nil {
		slog.Warn("authenticated but not registered — access denied",
			"email", user.Email, "uid", user.ID)
		writeError(w, http.StatusForbidden, "user not registered — contact your admin")
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

// extractBearerToken pulls the token from "Authorization: Bearer <token>".
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
