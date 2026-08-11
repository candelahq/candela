package auth

import (
	"context"
	"errors"
	"testing"
)

func TestGoogleOAuthResolver_Success(t *testing.T) {
	v := &mockAccessTokenValidator{
		validateFunc: func(ctx context.Context, token string) (*User, error) {
			return &User{ID: "id", Email: "user@example.com"}, nil
		},
	}
	resolver := NewGoogleOAuthResolver(v, nil)
	id, err := resolver.Resolve(context.Background(), "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Provider != "google-oauth" {
		t.Errorf("expected provider google-oauth, got %s", id.Provider)
	}
}

func TestGoogleOAuthResolver_SADenied(t *testing.T) {
	v := &mockAccessTokenValidator{
		validateFunc: func(ctx context.Context, token string) (*User, error) {
			return &User{ID: "id", Email: "sa@test.gserviceaccount.com"}, nil
		},
	}
	resolver := NewGoogleOAuthResolver(v, nil) // nil allowlist means deny all
	_, err := resolver.Resolve(context.Background(), "token")
	var forbiddenErr *ForbiddenError
	if !errors.As(err, &forbiddenErr) {
		t.Fatalf("expected ForbiddenError, got %v", err)
	}
}

func TestGoogleOAuthResolver_InvalidToken(t *testing.T) {
	v := &mockAccessTokenValidator{
		validateFunc: func(ctx context.Context, token string) (*User, error) {
			return nil, errors.New("invalid")
		},
	}
	resolver := NewGoogleOAuthResolver(v, nil)
	id, err := resolver.Resolve(context.Background(), "token")
	if id != nil || err != nil {
		t.Fatalf("expected (nil, nil) for invalid oauth token, got (%v, %v)", id, err)
	}
}
