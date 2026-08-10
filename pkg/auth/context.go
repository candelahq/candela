// Package auth provides authentication middleware and user context helpers
// for the Candela server. In production, Firebase Auth validates browser users
// and Google ID tokens validate candela-local/service account callers.
// The middleware extracts the email/uid and makes the user available via context.
package auth

import "context"

// User is a type alias for Identity, providing full backward compatibility.
// All existing code that references auth.User continues to compile and work.
// New code should prefer auth.Identity for clarity.
//
// This alias will be removed once all consumers have migrated to Identity.
type User = Identity

type contextKey struct{}

// NewContext returns a context with the given Identity attached.
func NewContext(ctx context.Context, u *Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// FromContext extracts the authenticated Identity from the context.
// Returns nil if no identity is present (e.g., unauthenticated request).
func FromContext(ctx context.Context) *Identity {
	u, _ := ctx.Value(contextKey{}).(*Identity)
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
