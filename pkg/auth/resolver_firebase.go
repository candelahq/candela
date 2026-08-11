package auth

import (
	"context"
	"fmt"
	"strings"
)

type FirebaseResolver struct {
	verifier TokenVerifier
}

func NewFirebaseResolver(verifier TokenVerifier) *FirebaseResolver {
	return &FirebaseResolver{verifier: verifier}
}

func (r *FirebaseResolver) Name() string { return "firebase" }
func (r *FirebaseResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	if r.verifier == nil {
		return nil, nil
	}
	decoded, err := r.verifier.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, nil // Not a Firebase token, try next
	}
	email, _ := decoded.Claims["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("firebase token missing email claim")
	}
	return &Identity{ID: decoded.UID, Email: strings.ToLower(email), Provider: "firebase", Claims: decoded.Claims}, nil
}
