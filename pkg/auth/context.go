// Package auth provides IAP-based authentication and user context helpers
// for the Candela server. In production, IAP validates the identity and sets
// a JWT assertion header. The interceptor extracts the email/sub and makes
// the user available via context.
package auth

import "context"

// User represents the authenticated identity for the current request.
type User struct {
	ID    string // IAP subject (stable identifier)
	Email string // IAP email claim
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
