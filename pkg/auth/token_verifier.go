package auth

import (
	"context"

	fbauth "firebase.google.com/go/v4/auth"
)

// TokenVerifier validates authentication tokens and returns the decoded payload.
// This interface abstracts *fbauth.Client.VerifyIDToken for testability.
//
// Implementations:
//   - *fbauth.Client (production, via FirebaseTokenVerifier adapter)
//   - MockTokenVerifier (tests)
type TokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error)
}

// FirebaseTokenVerifier adapts *fbauth.Client to the TokenVerifier interface.
type FirebaseTokenVerifier struct {
	client *fbauth.Client
}

// NewFirebaseTokenVerifier wraps a Firebase Auth client as a TokenVerifier.
// Returns nil (untyped) when client is nil to prevent the "typed nil" interface
// gotcha — a non-nil interface wrapping a nil pointer would bypass != nil checks
// and panic on VerifyIDToken.
func NewFirebaseTokenVerifier(client *fbauth.Client) TokenVerifier {
	if client == nil {
		return nil
	}
	return &FirebaseTokenVerifier{client: client}
}

// VerifyIDToken delegates to the underlying Firebase Auth client.
func (f *FirebaseTokenVerifier) VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error) {
	return f.client.VerifyIDToken(ctx, idToken)
}

// AccessTokenValidator validates OAuth2 access tokens and returns user info.
// This interface abstracts the validateAccessToken function for testability,
// preventing tests from making live network calls to Google's userinfo endpoint.
//
// Implementations:
//   - googleAccessTokenValidator (production, calls googleapis.com/oauth2/v3/userinfo)
//   - mockAccessTokenValidator (tests)
type AccessTokenValidator interface {
	ValidateAccessToken(ctx context.Context, accessToken string) (*User, error)
}

// googleAccessTokenValidator is the production implementation that calls
// Google's userinfo endpoint.
type googleAccessTokenValidator struct{}

// ValidateAccessToken delegates to the package-level validateAccessToken function.
func (g *googleAccessTokenValidator) ValidateAccessToken(ctx context.Context, accessToken string) (*User, error) {
	return validateAccessToken(ctx, accessToken)
}

// DefaultAccessTokenValidator returns the production validator that calls
// Google's OAuth2 userinfo endpoint.
func DefaultAccessTokenValidator() AccessTokenValidator {
	return &googleAccessTokenValidator{}
}
