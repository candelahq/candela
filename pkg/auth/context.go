// Package auth provides authentication middleware and user context helpers
// for the Candela server. In production, Firebase Auth validates browser users
// and Google ID tokens validate candela-local/service account callers.
// The middleware extracts the email/uid and makes the user available via context.
package auth

import (
	"context"
	"strings"
)

// User represents the authenticated identity for the current request.
type User struct {
	ID    string // Firebase UID or Google subject
	Email string // Verified email claim
}

// EffectiveID returns the canonical user identifier used for span attribution
// and budget tracking. Prefers lowercased email (matching the proxy's user_id
// convention) and falls back to the raw ID.
func (u *User) EffectiveID() string {
	if u.Email != "" {
		return strings.ToLower(u.Email)
	}
	return u.ID
}

type contextKey struct{}

// NewContext returns a context with the given User attached.
func NewContext(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// FromContext extracts the authenticated User from the context.
// Returns nil if no user is present (e.g., dev mode without IAP).
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(contextKey{}).(*User)
	return u
}

// IDFromContext returns the user ID from context, or empty string if absent.
func IDFromContext(ctx context.Context) string {
	if u := FromContext(ctx); u != nil {
		return u.ID
	}
	return ""
}

// EmailFromContext returns the user email from context, or empty string if absent.
func EmailFromContext(ctx context.Context) string {
	if u := FromContext(ctx); u != nil {
		return u.Email
	}
	return ""
}
