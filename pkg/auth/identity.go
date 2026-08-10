package auth

import (
	"context"
	"strings"
)

// Identity represents the authenticated caller for the current request.
// It extends the original User fields with provider metadata and tenant
// information, enabling cloud-agnostic identity resolution.
//
// Phase 1: Introduced as a superset of User. The type alias `User = Identity`
// in context.go ensures full backward compatibility with existing code.
type Identity struct {
	// ID is the unique identifier from the identity provider
	// (Firebase UID, Google "sub" claim, Zitadel user ID, etc.).
	// Named "ID" for backward compatibility with existing code that
	// references user.ID throughout the codebase.
	ID string

	// Email is the verified email claim. Primary identifier for RBAC
	// and span attribution.
	Email string

	// Provider identifies which IdentityResolver authenticated this identity.
	// Examples: "firebase", "google-oidc", "google-oauth", "oidc:https://zitadel.example.com", "dev".
	Provider string

	// TenantIDs lists the tenants this identity is authorized for.
	// Populated from IdP claims (e.g., Zitadel custom claims) or user store.
	// Empty means "no tenant restriction" in TenantModeOpen.
	TenantIDs []string

	// Claims holds the raw token claims for extensibility.
	// Resolvers populate this with the decoded JWT claims or userinfo response.
	Claims map[string]any
}

// EffectiveID returns the canonical user identifier used for span attribution
// and budget tracking. Prefers lowercased email (matching the proxy's user_id
// convention) and falls back to the raw ID.
func (i *Identity) EffectiveID() string {
	if i.Email != "" {
		return strings.ToLower(i.Email)
	}
	return i.ID
}

// IdentityResolver attempts to resolve a bearer token into an Identity.
//
// Contract:
//   - Returns (identity, nil) on successful authentication.
//   - Returns (nil, nil) if the token format is not recognized by this resolver.
//     The chain should try the next resolver.
//   - Returns (nil, err) if the token IS recognized but invalid (expired,
//     tampered, revoked). The chain should reject the request.
type IdentityResolver interface {
	// Name returns a human-readable name for logging (e.g., "firebase", "oidc:zitadel").
	Name() string

	// Resolve validates the token and extracts an Identity.
	Resolve(ctx context.Context, token string) (*Identity, error)
}
