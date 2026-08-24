package auth

import (
	"errors"
	"log/slog"
	"net/http"
)

// ChainAuthMiddleware validates bearer tokens using a pre-built ResolverChain.
// This is the config-driven alternative to FirebaseAuthMiddleware — it works
// with any combination of resolvers (OIDC, Firebase, Google, etc.).
//
// The middleware:
//  1. Skips health-check paths (/healthz, /readyz)
//  2. Extracts the bearer token from Authorization or X-Candela-Auth headers
//  3. Resolves the token through the chain (first match wins)
//  4. Verifies the user is registered (unless self-service path)
//  5. Injects the Identity into the request context
func ChainAuthMiddleware(next http.Handler, chain *ResolverChain, userAuth UserAuthorizer, saAllowlist *ServiceAccountAllowlist, devMode bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks (liveness + readiness).
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		isSelfService := selfServicePaths[r.URL.Path]

		token := extractBearerToken(r)
		if token == "" {
			if devMode {
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

		if !isSelfService && !verifyRegistered(r.Context(), w, user, userAuth, saAllowlist) {
			return
		}

		slog.Debug("authenticated", "provider", user.Provider, "uid", user.ID, "email", user.Email, "path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), user)))
	})
}
