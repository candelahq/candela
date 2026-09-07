package auth

import (
	"context"
	"errors"
	"testing"

	fbauth "firebase.google.com/go/v4/auth"
)

func TestFirebaseResolver_Success(t *testing.T) {
	v := &mockTokenVerifier{
		verifyFunc: func(ctx context.Context, idToken string) (*fbauth.Token, error) {
			return &fbauth.Token{UID: "uid", Claims: map[string]interface{}{
				"email":          "USER@example.com",
				"email_verified": true,
			}}, nil
		},
	}
	resolver := NewFirebaseResolver(v)
	id, err := resolver.Resolve(context.Background(), "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Email != "user@example.com" {
		t.Errorf("expected lowercase email, got %s", id.Email)
	}
	if id.Provider != "firebase" {
		t.Errorf("expected provider firebase, got %s", id.Provider)
	}
}

func TestFirebaseResolver_UnverifiedEmail(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]interface{}
	}{
		{
			name: "explicitly false",
			claims: map[string]interface{}{
				"email":          "user@example.com",
				"email_verified": false,
			},
		},
		{
			name: "missing email_verified claim",
			claims: map[string]interface{}{
				"email": "user@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &mockTokenVerifier{
				verifyFunc: func(ctx context.Context, idToken string) (*fbauth.Token, error) {
					return &fbauth.Token{UID: "uid", Claims: tt.claims}, nil
				},
			}
			resolver := NewFirebaseResolver(v)
			_, err := resolver.Resolve(context.Background(), "token")
			if err == nil || err.Error() != "firebase email not verified" {
				t.Fatalf("expected 'firebase email not verified' error, got %v", err)
			}
		})
	}
}

func TestFirebaseResolver_MissingEmail(t *testing.T) {
	v := &mockTokenVerifier{
		verifyFunc: func(ctx context.Context, idToken string) (*fbauth.Token, error) {
			return &fbauth.Token{UID: "uid", Claims: map[string]interface{}{}}, nil
		},
	}
	resolver := NewFirebaseResolver(v)
	_, err := resolver.Resolve(context.Background(), "token")
	if err == nil || err.Error() != "firebase token missing email claim" {
		t.Fatalf("expected missing email error, got %v", err)
	}
}

func TestFirebaseResolver_InvalidToken(t *testing.T) {
	v := &mockTokenVerifier{
		verifyFunc: func(ctx context.Context, idToken string) (*fbauth.Token, error) {
			return nil, errors.New("invalid token")
		},
	}
	resolver := NewFirebaseResolver(v)
	id, err := resolver.Resolve(context.Background(), "token")
	if id != nil || err != nil {
		t.Fatalf("expected (nil, nil) for invalid firebase token, got (%v, %v)", id, err)
	}
}
