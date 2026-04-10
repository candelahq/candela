package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"
)

// IAPMiddleware validates the IAP JWT assertion header on every request and
// injects the authenticated user into the request context.
//
// In dev mode (devMode=true), no JWT validation is performed; a synthetic
// admin user is injected instead.
//
// Header: x-goog-iap-jwt-assertion (set by IAP automatically)
func IAPMiddleware(next http.Handler, audience string, devMode bool) http.Handler {
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

		// Production: validate IAP JWT.
		assertion := r.Header.Get("x-goog-iap-jwt-assertion")
		if assertion == "" {
			slog.Warn("missing IAP JWT assertion", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "missing authentication")
			return
		}

		payload, err := idtoken.Validate(r.Context(), assertion, audience)
		if err != nil {
			slog.Warn("invalid IAP JWT", "error", err, "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "invalid authentication token")
			return
		}

		// Extract email and subject from the validated payload.
		email, _ := payload.Claims["email"].(string)
		sub := payload.Subject
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
