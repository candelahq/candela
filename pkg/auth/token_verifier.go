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
func NewFirebaseTokenVerifier(client *fbauth.Client) *FirebaseTokenVerifier {
	return &FirebaseTokenVerifier{client: client}
}

// VerifyIDToken delegates to the underlying Firebase Auth client.
func (f *FirebaseTokenVerifier) VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error) {
	return f.client.VerifyIDToken(ctx, idToken)
}
