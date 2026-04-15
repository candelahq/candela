package auth

import (
	"log/slog"
	"net/http"
	"strings"

	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/idtoken"
)

// FirebaseAuthMiddleware validates Firebase ID tokens (from browser users) and
// Google ID tokens (from candela-local / service accounts).
//
// Auth flow:
//   - Browser: Firebase Auth → ID token in Authorization: Bearer header
//   - candela-local: Cloud Run invoker IAM → Google ID token in Authorization header
//
// In dev mode (devMode=true), no validation is performed; a synthetic admin
// user is injected instead.
func FirebaseAuthMiddleware(next http.Handler, fbAuth *fbauth.Client, cloudRunAudience string, devMode bool) http.Handler {
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
				slog.Debug("authenticated via Google ID token",
					"uid", user.ID, "email", user.Email, "path", r.URL.Path)
				next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
				return
			}
			slog.Warn("Google ID token validation failed",
				"error", err, "path", r.URL.Path)
		}

		writeError(w, http.StatusUnauthorized, "invalid authentication token")
	})
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
