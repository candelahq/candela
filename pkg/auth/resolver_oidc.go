package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCClaimMapping configures how OIDC token claims map to Identity fields.
// Defaults work for most providers (Zitadel, Auth0, Okta, Keycloak).
type OIDCClaimMapping struct {
	Subject string // default: "sub"
	Email   string // default: "email"
	Roles   string // default: "roles" (optional)
	Tenants string // default: "candela.tenants" (optional)
}

// DefaultOIDCClaimMapping returns sensible defaults.
func DefaultOIDCClaimMapping() OIDCClaimMapping {
	return OIDCClaimMapping{
		Subject: "sub",
		Email:   "email",
		Roles:   "roles",
		Tenants: "candela.tenants",
	}
}

// OIDCResolver validates tokens from any OIDC-compliant identity provider.
// It auto-discovers the JWKS endpoint from the issuer's .well-known/openid-configuration.
type OIDCResolver struct {
	issuer   string
	verifier *oidc.IDTokenVerifier
	claimMap OIDCClaimMapping
}

// NewOIDCResolver creates a resolver for the given OIDC issuer.
// It performs OIDC discovery to find the JWKS endpoint.
// The audience is the expected "aud" claim in the token.
func NewOIDCResolver(ctx context.Context, issuer, audience string, claimMap OIDCClaimMapping) (*OIDCResolver, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", issuer, err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: audience})
	return &OIDCResolver{
		issuer:   issuer,
		verifier: verifier,
		claimMap: claimMap,
	}, nil
}

func (r *OIDCResolver) Name() string { return "oidc:" + r.issuer }

func (r *OIDCResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	idToken, err := r.verifier.Verify(ctx, token)
	if err != nil {
		// If the error indicates wrong issuer or audience, this isn't our token.
		// Otherwise it IS our token but invalid.
		errMsg := err.Error()
		if strings.Contains(errMsg, "oidc: id token issued by a different provider") ||
			strings.Contains(errMsg, "oidc: expected audience") {
			return nil, nil // Not our token, try next resolver
		}
		return nil, fmt.Errorf("%s: %w", r.Name(), err) // Our token, but invalid
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%s: failed to parse claims: %w", r.Name(), err)
	}

	email := extractClaimString(claims, r.claimMap.Email)
	if email == "" {
		return nil, fmt.Errorf("%s: token missing email claim", r.Name())
	}

	subject := extractClaimString(claims, r.claimMap.Subject)
	if subject == "" {
		subject = idToken.Subject
	}

	return &Identity{
		ID:        subject,
		Email:     strings.ToLower(email),
		Provider:  "oidc:" + r.issuer,
		TenantIDs: extractClaimStringSlice(claims, r.claimMap.Tenants),
		Claims:    claims,
	}, nil
}

// extractClaimString gets a string value from a claims map.
func extractClaimString(claims map[string]interface{}, key string) string {
	if key == "" {
		return ""
	}
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// extractClaimStringSlice gets a []string from a claims map.
// Handles both []string and []interface{} (common in JWT claims).
func extractClaimStringSlice(claims map[string]interface{}, key string) []string {
	if key == "" {
		return nil
	}
	v, ok := claims[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}
